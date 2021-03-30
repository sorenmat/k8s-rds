package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sorenmat/k8s-rds/client"
	"github.com/sorenmat/k8s-rds/crd"
	"github.com/sorenmat/k8s-rds/kube"
	"github.com/sorenmat/k8s-rds/local"
	"github.com/sorenmat/k8s-rds/provider"
	"github.com/sorenmat/k8s-rds/rds"
	"github.com/spf13/cobra"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"k8s.io/client-go/tools/clientcmd"
)

const Failed = "Failed"

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
		config, err = clientcmd.BuildConfigFromFlags("", kube.Config())
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

func main() {
	var (
		_provider         string
		excludeNamespaces []string
		includeNamespaces []string
		repository        string
	)
	var rootCmd = &cobra.Command{
		Use:   "k8s-rds",
		Short: "Kubernetes database provisioner",
		Long:  `Kubernetes database provisioner`,
		Run: func(cmd *cobra.Command, args []string) {
			execute(_provider, excludeNamespaces, includeNamespaces, repository)
		},
	}
	rootCmd.PersistentFlags().StringVar(&_provider, "provider", "aws", "Type of provider (aws, local)")
	rootCmd.PersistentFlags().StringSliceVar(&excludeNamespaces, "exclude-namespaces", nil, "list of namespaces to exclude. Mutually exclusive with --include-namespaces.")
	rootCmd.PersistentFlags().StringSliceVar(&includeNamespaces, "include-namespaces", nil, "list of namespaces to include. Mutually exclusive with --exclude-namespaces.")
	rootCmd.PersistentFlags().StringVar(&repository, "repository", "", "Docker image repository, default is hub.docker.com)")
	if len(excludeNamespaces) > 0 && len(includeNamespaces) > 0 {
		panic("--include-namespaces and --exclude-namespaces are mutually exclusive")
	}
	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}

func execute(dbprovider string, excludeNamespaces, includeNamespaces []string, repository string) {
	log.Println("Starting k8s-rds")

	config, err := getClientConfig(kube.Config())
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
				if excluded(db, excludeNamespaces, includeNamespaces) {
					return
				}
				_client := client.CrdClient(crdcs, scheme, db.Namespace) // add the database namespace to the client
				err = handleCreateDatabase(context.Background(), db, _client, dbprovider, repository)
				if err != nil {
					log.Printf("database creation failed: %v", err)
					err := updateStatus(db, crd.DatabaseStatus{Message: fmt.Sprintf("%v", err), State: Failed}, _client)
					if err != nil {
						log.Printf("database CRD status update failed: %v", err)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				if excluded(db, excludeNamespaces, includeNamespaces) {
					return
				}
				log.Printf("deleting database: %s \n", db.Name)

				r, err := getProvider(db, dbprovider, repository)
				if err != nil {
					log.Println(err)
					return
				}

				err = r.DeleteDatabase(context.Background(), db)
				if err != nil {
					log.Println(err)
				}

				err = r.DeleteService(db.Namespace, db.Name)
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

func getProvider(db *crd.Database, dbprovider, repository string) (provider.DatabaseProvider, error) {
	kubectl, err := getKubectl()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	_provider := dbprovider
	if db.Spec.Provider != "" {
		_provider = db.Spec.Provider
	}
	switch _provider {
	case "aws":
		r, err := rds.New(context.Background(), db, kubectl)
		if err != nil {
			return nil, err
		}
		return r, nil

	case "local":
		r, err := local.New(db, kubectl, repository)
		if err != nil {
			return nil, err
		}
		return r, nil
	}
	return nil, fmt.Errorf("unable to find provider for %v", dbprovider)
}

func handleCreateDatabase(ctx context.Context, db *crd.Database, crdclient *client.Crdclient, dbprovider, repository string) error {
	// we don't need to skip when it is a local provider without running pod
	if db.Status.State == "Created" && dbprovider == "aws" {
		log.Printf("database %v already created, skipping\n", db.Name)
		return nil
	}
	// validate dbname is only alpha numeric
	// This check is needed in case local provider with already created db
	if db.Status.State != "Created" {
		err := updateStatus(db, crd.DatabaseStatus{Message: "Creating", State: "Creating"}, crdclient)
		if err != nil {
			return fmt.Errorf("database CRD status update failed: %v", err)
		}
	}

	log.Println("trying to get kubectl")

	r, err := getProvider(db, dbprovider, repository)
	if err != nil {
		return err
	}

	hostname, err := r.CreateDatabase(ctx, db)
	if err != nil {
		return err
	}

	log.Printf("Creating service '%v' for %v\n", db.Name, hostname)
	err = r.CreateService(db.Namespace, hostname, db.Name)
	if err != nil {
		return err
	}

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
	_, err = crdclient.Update(db)
	if err != nil {
		return err
	}
	return nil
}

func excluded(db *crd.Database, excludeNamespaces, includeNamespaces []string) bool {
	if len(excludeNamespaces) > 0 && stringInSlice(db.Namespace, excludeNamespaces) {
		log.Printf("database %s is in excluded namespace %s. Ignoring...", db.Name, db.Namespace)
		return true
	}
	if len(includeNamespaces) > 0 && !stringInSlice(db.Namespace, includeNamespaces) {
		log.Printf("database %s is in a non included namespace %s. Ignoring...", db.Name, db.Namespace)
		return true
	}
	return false
}

func stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if str == s {
			return true
		}
	}
	return false
}
