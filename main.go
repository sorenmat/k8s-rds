package main

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
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

const Failed = "Failed"

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
		Region: region,
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
	log.Printf("Found node with ID: %v in region %v", name, region)

	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	// Set the AWS Region that the service clients should use
	cfg.Region = region
	cfg.HTTPClient.Timeout = 5 * time.Second
	return ec2.New(cfg), nil

}

// getSubnets returns a list of subnets that the RDS instance should be attached to
// We do this by finding a node in the cluster, take the VPC id from that node a list
// the security groups in the VPC
func getSubnets(svc *ec2.EC2, public bool) ([]string, error) {
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
	log.Printf("Taking subnets from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	req := svc.DescribeInstancesRequest(params)
	res, err := req.Send()
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	log.Println("got instance response")

	var result []string
	if len(res.Reservations) >= 1 {
		vpcID := res.Reservations[0].Instances[0].VpcId
		for _, v := range res.Reservations[0].Instances[0].SecurityGroups {
			log.Println("Security groupid: ", *v.GroupId)
		}
		log.Printf("Found VPC %v will search for subnet in that VPC\n", *vpcID)

		res := svc.DescribeSubnetsRequest(&ec2.DescribeSubnetsInput{Filters: []ec2.Filter{{Name: aws.String("vpc-id"), Values: []string{*vpcID}}}})
		subnets, err := res.Send()

		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("unable to describe subnet in VPC %v", *vpcID))
		}
		for _, sn := range subnets.Subnets {
			if *sn.MapPublicIpOnLaunch == public {
				result = append(result, *sn.SubnetId)
			} else {
				log.Printf("Skipping subnet %v since it's public state was %v and we were looking for %v\n", *sn.SubnetId, *sn.MapPublicIpOnLaunch, public)
			}
		}

	}
	log.Printf("Found the follwing subnets: ")
	for _, v := range result {
		log.Printf(v + " ")
	}
	return result, nil
}

func getSGS(svc *ec2.EC2) ([]string, error) {
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
	log.Printf("Taking security groups from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	req := svc.DescribeInstancesRequest(params)
	res, err := req.Send()
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	log.Println("got instance response")

	var result []string
	if len(res.Reservations) >= 1 {
		for _, v := range res.Reservations[0].Instances[0].SecurityGroups {
			fmt.Println("Security groupid: ", *v.GroupId)
			result = append(result, *v.GroupId)
		}
	}

	log.Printf("Found the follwing security groups: ")
	for _, v := range result {
		log.Printf(v + " ")
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
		log.Fatal("unable to create a client for EC2 ", err)
	}
	// Create a CRD client interface
	crdclient := client.CrdClient(crdcs, scheme, "")
	log.Println("Watching for database changes...")
	_, controller := cache.NewInformer(
		crdclient.NewListWatch(),
		&crd.Database{},
		time.Minute*2,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				client := client.CrdClient(crdcs, scheme, db.Namespace) // add the database namespace to the client
				err = handleCreateDatabase(db, ec2client, client)
				if err != nil {
					log.Printf("database creation failed: %v", err)
					err := updateStatus(db, crd.DatabaseStatus{Message: fmt.Sprintf("%v", err), State: Failed}, client)
					if err != nil {
						log.Printf("database CRD status update failed: %v", err)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				log.Printf("deleting database: %s \n", db.Name)
				subnets := db.Spec.Subnets
				if db.Spec.DBSubnetGroupName == "" && len(subnets) < 2 {
					log.Println("trying to get subnets")
					subnets, err = getSubnets(ec2client, db.Spec.PubliclyAccessible)
					if err != nil {
						log.Println(err)
					}
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

func handleCreateDatabase(db *crd.Database, ec2client *ec2.EC2, crdclient *client.Crdclient) error {
	if db.Status.State == "Created" {
		log.Printf("database %v already created, skipping\n", db.Name)
		return nil
	}
	// validate dbname is only alpha numeric
	err := updateStatus(db, crd.DatabaseStatus{Message: "Creating", State: "Creating"}, crdclient)
	if err != nil {
		return fmt.Errorf("database CRD status update failed: %v", err)
	}
	subnets := db.Spec.Subnets
	if db.Spec.DBSubnetGroupName == "" && len(subnets) < 2 {
		log.Println("trying to get subnets")
		subnets, err = getSubnets(ec2client, db.Spec.PubliclyAccessible)
		if err != nil {
			return fmt.Errorf("unable to get subnets from instance: %v", err)
		}
	}
	sgs := db.Spec.SecurityGroups
	if len(sgs) == 0 {
		log.Println("trying to get security groups")
		sgs, err = getSGS(ec2client)
		if err != nil {
			return fmt.Errorf("unable to get security groups from instance: %v", err)
		}
	}

	r := rds.RDS{EC2: ec2client, Subnets: subnets, SecurityGroups: sgs}
	log.Println("trying to get kubectl")
	kubectl, err := getKubectl()
	if err != nil {
		return err
	}

	k := kube.Kube{Client: kubectl}

	pw := ""
	if db.Spec.Password.Name != "" && db.Spec.Password.Key != "" {
		log.Printf("getting secret: Name: %v Key: %v \n", db.Spec.Password.Name, db.Spec.Password.Key)
		pw, err = k.GetSecret(db.Namespace, db.Spec.Password.Name, db.Spec.Password.Key)
		if err != nil {
			return err
		}
	}
	hostname, err := r.CreateDatabase(db, pw)
	if err != nil {
		return err
	}
	log.Printf("Creating service '%v' for %v\n", db.Name, hostname)
	k.CreateService(db.Namespace, hostname, db.Name)

	err = updateStatus(db, crd.DatabaseStatus{Message: "Created", State: "Created"}, crdclient)
	if err != nil {
		return err
	}
	log.Printf("Creation of database %v done\n", db.Name)
	return nil
}

func updateStatus(db *crd.Database, status crd.DatabaseStatus, crdclient *client.Crdclient) error {
	db, err := crdclient.Get(db.Name)
	if err != nil {
		return err
	}

	db.Status = status
	db, err = crdclient.Update(db)
	if err != nil {
		return err
	}
	return nil
}
