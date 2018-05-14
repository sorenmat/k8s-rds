package main

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/client"
	"github.com/sorenmat/k8s-rds/crd"
	"github.com/sorenmat/k8s-rds/kube"
	"github.com/sorenmat/k8s-rds/rds"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func home() string {
	dir, err := homedir.Dir()
	home, err := homedir.Expand(dir)
	if err != nil {
		panic(err.Error())
	}
	return home
}

func kubeconfig() string {
	return home() + "/.kube/config"
}

// return rest config, if path not specified assume in cluster config
func getClientConfig(kubeconfig string) (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		if kubeconfig != "" {
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}
	return cfg, err
}

func getKubectl() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Println("Appears we are not running in a cluster")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig())
		if err != nil {
			return nil, err
		}
	} else {
		log.Println("Seems like we are running in a Kubernetes cluster!!")
	}

	kubectl, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubectl, nil
}

func ec2config(region string) *aws.Config {
	return &aws.Config{
		Region: aws.String(region),
		CredentialsChainVerboseErrors: aws.Bool(true),
	}
}

func ec2client() (*ec2.EC2, error) {
	kubectl, err := getKubectl()
	if err != nil {
		return nil, err
	}

	nodes, err := kubectl.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""
	region := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = nodes.Items[0].Spec.ExternalID
		region = nodes.Items[0].Labels["failure-domain.beta.kubernetes.io/region"]
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Println("Found node with ID: ", name)

	ses, err := session.NewSession(ec2config(region))
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	svc := ec2.New(ses, ec2config(region))
	return svc, nil
}

// getSubnets returns a list of subnets that the RDS instance should be attached to
// We do this by findind a node in the cluster, take the VPC id from that node a list
// the security groups in the VPC
func getSubnets(public bool) ([]*string, error) {
	kubectl, err := getKubectl()
	if err != nil {
		return nil, err
	}

	nodes, err := kubectl.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = nodes.Items[0].Spec.ExternalID
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Println("Found node with ID: ", name)

	svc, err := ec2client()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get EC2 client")
	}

	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []*string{
					aws.String(name),
				},
			},
		},
	}

	res, err := svc.DescribeInstances(params)
	if err != nil {
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	var result []*string
	if len(res.Reservations) >= 1 {
		vpcID := res.Reservations[0].Instances[0].VpcId
		log.Printf("Found VPC %v will search for subnet in that VPC\n", *vpcID)
		subnets, err := svc.DescribeSubnets(&ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{{Name: aws.String("vpc-id"), Values: []*string{vpcID}}}})
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("unable to describe subnet in VPC %v", *vpcID))
		}
		for _, sn := range subnets.Subnets {
			if *sn.MapPublicIpOnLaunch == public {
				result = append(result, sn.SubnetId)
			} else {
				log.Printf("Skipping subnet %v since it's public state was %v and we were looking for %v\n", sn.SubnetId, *sn.MapPublicIpOnLaunch, public)
			}
		}

	}
	log.Printf("Found the follwing subnets: ")
	for _, v := range result {
		log.Printf(*v + " ")
	}
	return result, nil
}

func main() {
	log.Println("Starting k8s-rds")
	config, err := getClientConfig(kubeconfig())
	if err != nil {
		panic(err.Error())
	}

	// create clientset and create our CRD, this only need to run once
	clientset, err := apiextcs.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// note: if the CRD exist our CreateCRD function is set to exit without an error
	err = crd.CreateCRD(clientset)
	if err != nil {
		panic(err)
	}

	// Create a new clientset which include our CRD schema
	crdcs, scheme, err := crd.NewClient(config)
	if err != nil {
		panic(err)
	}

	ec2client, err := ec2client()
	if err != nil {
		log.Fatal("unable to create a client for EC2")
	}
	// Create a CRD client interface
	crdclient := client.CrdClient(crdcs, scheme, "default")

	log.Println("Watching for database changes...")
	_, controller := cache.NewInformer(
		crdclient.NewListWatch(),
		&crd.Database{},
		time.Minute*2,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				// validate dbname is only alpha numeric
				db.Status = crd.DatabaseStatus{Message: "Creating", State: "Creating"}
				db, err = crdclient.Update(db)
				if err == nil {
					log.Println("Database CRD status updated")
				} else {
					log.Println("Database CRD status update failed: ", err)
				}
				subnets, err := getSubnets(db.Spec.PubliclyAccessible)
				if err != nil {
					log.Println(err)
				}

				r := rds.RDS{EC2: ec2client, Subnets: subnets}
				kubectl, err := getKubectl()
				if err != nil {
					log.Println(err)
					return
				}

				k := kube.Kube{Client: kubectl}

				pw, err := k.GetSecret(db.Namespace, db.Spec.Password.Name, db.Spec.Password.Key)
				if err != nil {
					log.Println(err)
				}
				hostname, err := r.CreateDatabase(db, crdclient, pw)
				if err != nil {
					log.Println(err)
				}
				log.Printf("Creating service '%v' for %v\n", db.Name, hostname)
				k.CreateService(db.Namespace, hostname, db.Name)
				db.Status = crd.DatabaseStatus{Message: "Created", State: "Created"}
				db, err = crdclient.Update(db)
				if err != nil {
					log.Println(err)
				}
				log.Printf("Creation of database %v done\n", db.Name)
			},
			DeleteFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				log.Printf("deleting database: %s \n", db.Name)
				subnets, err := getSubnets(db.Spec.PubliclyAccessible)
				if err != nil {
					log.Println(err)
				}
				r := rds.RDS{EC2: ec2client, Subnets: subnets}
				r.DeleteDatabase(db)
				kubectl, err := getKubectl()
				if err != nil {
					log.Println(err)
					return
				}
				k := kube.Kube{Client: kubectl}
				err = k.DeleteService(db.Namespace, db.Name)
				if err != nil {
					log.Println(err)
				}
				log.Printf("Deletion of database %v done\n", db.Name)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)

	// Wait forever
	select {}
}
