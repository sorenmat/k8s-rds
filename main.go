package main

import (
	"fmt"
	"github.com/sorenmat/k8s-rds/dbprovider"
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

func getDBProvider() (dbprovider.DBProvider,error) {
	if inMinikube() {
		return dbprovider.ContainerDBProvider{},nil
	} else {
		ec2client, err := ec2client()
		if err != nil {
			log.Fatal("unable to create a client for EC2")
		}

		return dbprovider.AWSDBProvider{
			Client: ec2client,
		},nil
	}
}
func inMinikube() bool {
	return false
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

	dbprovider := getDBProvider()

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
				go handleCreateDatabase(db, dbprovider, crdclient)
			},
			DeleteFunc: func(obj interface{}) {
				db := obj.(*crd.Database)

				kubectl, err := getKubectl()
				if err != nil {
					log.Println(err)
					return
				}

				// AWS Stuff
				err = dbprovider.DeleteDatabase(kubectl, db)
				// AWS End

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

func handleCreateDatabase(db *crd.Database, dbprovider dbprovider.DBProvider, crdclient *client.Crdclient) {
	if db.Status.State == "Created" {
		log.Printf("Database %v already created, skipping...", db.Name)
		return
	}
	// validate dbname is only alpha numeric
	db.Status = crd.DatabaseStatus{Message: "Creating", State: "Creating"}
	db, err := crdclient.Update(db)
	if err == nil {
		log.Println("Database CRD status updated")
	} else {
		log.Println("Database CRD status update failed: ", err)
	}

	log.Println("trying to get kubectl")
	kubectl, err := getKubectl()
	if err != nil {
		log.Println(err)
		return
	}
	k := kube.Kube{Client: kubectl}
	log.Println("getting secret")
	pw, err := k.GetSecret(db.Namespace, db.Spec.Password.Name, db.Spec.Password.Key)
	if err != nil {
		log.Println(err)
	}

	// AWS Stuff
	hostname,err := dbprovider.CreateDatabase(kubectl, db)
	// End of AWS

	log.Printf("Creating service '%v' for %v\n", db.Name, hostname)
	k.CreateService(db.Namespace, hostname, db.Name)
	db.Status = crd.DatabaseStatus{Message: "Created", State: "Created"}
	db, err = crdclient.Update(db)
	if err != nil {
		log.Println(err)
	}
	log.Printf("Creation of database %v done\n", db.Name)
}
