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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	err = crd.CreateDBClusterCRD(clientset)
	if err != nil {
		panic(err)
	}

	// Create a new clientset which include our CRD schema
	crdcs, scheme, err := crd.NewClient(config)
	if err != nil {
		panic(err)
	}
	crdcsCluster, clusterScheme, err := crd.NewDBClusterClient(config)
	if err != nil {
		panic(err)
	}

	// Create a CRD client interface
	crdclient := client.CrdClient(crdcs, scheme, "")
	clusterCrdclient := client.DBClusterCrdClient(crdcsCluster, clusterScheme, "")
	log.Println("Watching for database changes...")
	_, controller := cache.NewInformer(
		crdclient.NewListWatch(),
		&crd.Database{},
		time.Minute*2,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				if excluded(db.Namespace, db.Name, excludeNamespaces, includeNamespaces) {
					return
				}
				_client := client.CrdClient(crdcs, scheme, db.Namespace) // add the database namespace to the client
				err = handleCreateDatabase(context.Background(), db, _client, dbprovider, repository)
				if err != nil {
					log.Printf("database creation failed: %v", err)
					err := updateStatus(context.Background(), db, crd.DatabaseStatus{Message: fmt.Sprintf("%v", err), State: Failed}, _client)
					if err != nil {
						log.Printf("database CRD status update failed: %v", err)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				ctx := context.Background()

				db := obj.(*crd.Database)
				if excluded(db.Namespace, db.Name, excludeNamespaces, includeNamespaces) {
					return
				}
				log.Printf("deleting database: %s \n", db.Name)

				r, err := getProvider(db.Spec.Provider, db.Spec.PubliclyAccessible, dbprovider, repository)
				if err != nil {
					log.Println(err)
					return
				}

				err = r.DeleteDatabase(ctx, db)
				if err != nil {
					log.Println(err)
				}

				err = r.DeleteService(ctx, db.Namespace, db.Name)
				if err != nil {
					log.Println(err)
				}
				log.Printf("Deletion of database %v done\n", db.Name)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldDB := oldObj.(*crd.Database)
				newDB := newObj.(*crd.Database)
				if excluded(newDB.Namespace, newDB.Name, excludeNamespaces, includeNamespaces) {
					return
				}
				// only update when one of these params changed
				if newDB.Spec.ApplyImmediately == oldDB.Spec.ApplyImmediately && newDB.Spec.Class == oldDB.Spec.Class && newDB.Spec.Size == oldDB.Spec.Size && newDB.Spec.MaxAllocatedSize == oldDB.Spec.MaxAllocatedSize {
					return
				}

				log.Printf("Updating database, class: %s, old class: %s, size: %d, old size: %d, maxAllocatedSize: %d, old maxAllocatedSize: %d", newDB.Spec.Class, oldDB.Spec.Class, newDB.Spec.Size, oldDB.Spec.Size, newDB.Spec.MaxAllocatedSize, oldDB.Spec.MaxAllocatedSize)
				_client := client.CrdClient(crdcs, scheme, newDB.Namespace) // add the database namespace to the client
				err = handleUpdateDatabase(context.Background(), newDB, _client, dbprovider, repository)

				if err != nil {
					log.Printf("database update failed: %v", err)
					err := updateStatus(context.Background(), newDB, crd.DatabaseStatus{Message: fmt.Sprintf("%v", err), State: Failed}, _client)
					if err != nil {
						log.Printf("database CRD status update failed: %v", err)
					}
				}
			},
		},
	)
	_, clusterController := cache.NewInformer(
		clusterCrdclient.NewListWatch(),
		&crd.DBCluster{},
		time.Minute*2,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				cluster := obj.(*crd.DBCluster)
				if excluded(cluster.Namespace, cluster.Name, excludeNamespaces, includeNamespaces) {
					return
				}
				_client := client.DBClusterCrdClient(crdcsCluster, scheme, cluster.Namespace) // add the database namespace to the client
				err = handleCreateDBCluster(context.Background(), cluster, _client, dbprovider, repository)
				if err != nil {
					log.Printf("db cluster creation failed: %v", err)
					err := updateClusterStatus(context.Background(), cluster, crd.DBClusterStatus{Message: fmt.Sprintf("%v", err), State: Failed}, _client)
					if err != nil {
						log.Printf("db cluster CRD status update failed: %v", err)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				ctx := context.Background()

				cluster := obj.(*crd.DBCluster)
				if excluded(cluster.Namespace, cluster.Name, excludeNamespaces, includeNamespaces) {
					return
				}
				log.Printf("deleting db cluster: %s \n", cluster.Name)
				publiclyAccessible := false
				if cluster.Spec.PubliclyAccessible != nil {
					publiclyAccessible = *cluster.Spec.PubliclyAccessible
				}
				r, err := getProvider(cluster.Spec.Provider, publiclyAccessible, dbprovider, repository)
				if err != nil {
					log.Println(err)
					return
				}
				crdclient := client.CrdClient(crdcs, scheme, cluster.Namespace)
				dbs, err := crdclient.List(ctx, v1.ListOptions{})
				if err == nil {
					// Delete DBs first
					for _, db := range dbs.Items {
						if db.Spec.DBClusterIdentifier == cluster.Spec.DBClusterIdentifier {
							log.Printf("found database %s child of deleted cluster:%s, deleting it\n", db.Name, cluster.Name)
							err := crdclient.Delete(ctx, db.Name, &v1.DeleteOptions{})
							if err != nil {
								log.Printf("deleting database %s child of deleted cluster:%s, failed:%v\n", db.Name, cluster.Name, err)
							}
						} else {
							log.Printf("database not a child of deleted cluster:%s\n", db.Name)
						}
					}
				} else {
					log.Printf("listing databases failed:%v\n", err)
				}
				err = r.DeleteDBCluster(ctx, cluster)
				if err != nil {
					log.Println(err)
				}

				err = r.DeleteService(ctx, cluster.Namespace, cluster.Name)
				if err != nil {
					log.Println(err)
				}
				log.Printf("Deletion of db cluster %v done\n", cluster.Name)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldDBCluster := oldObj.(*crd.DBCluster)
				newDBCluster := newObj.(*crd.DBCluster)
				if excluded(newDBCluster.Namespace, newDBCluster.Name, excludeNamespaces, includeNamespaces) {
					return
				}
				// only update when one of these params changed
				if newDBCluster.Spec.ApplyImmediately == oldDBCluster.Spec.ApplyImmediately &&
					newDBCluster.Spec.DBClusterInstanceClass == oldDBCluster.Spec.DBClusterInstanceClass &&
					newDBCluster.Spec.AllocatedStorage == oldDBCluster.Spec.AllocatedStorage &&
					newDBCluster.Spec.Port == oldDBCluster.Spec.Port &&
					newDBCluster.Spec.DeletionProtection == oldDBCluster.Spec.DeletionProtection &&
					(newDBCluster.Spec.ServerlessV2ScalingConfiguration == oldDBCluster.Spec.ServerlessV2ScalingConfiguration ||
						newDBCluster.Spec.ServerlessV2ScalingConfiguration != nil && oldDBCluster.Spec.ServerlessV2ScalingConfiguration != nil &&
							*newDBCluster.Spec.ServerlessV2ScalingConfiguration.MaxCapacity == *oldDBCluster.Spec.ServerlessV2ScalingConfiguration.MaxCapacity &&
							*newDBCluster.Spec.ServerlessV2ScalingConfiguration.MinCapacity == *oldDBCluster.Spec.ServerlessV2ScalingConfiguration.MinCapacity) {
					return
				}

				_client := client.DBClusterCrdClient(crdcsCluster, scheme, newDBCluster.Namespace)
				err = handleUpdateDBCluster(context.Background(), newDBCluster, _client, dbprovider, repository)

				if err != nil {
					log.Printf("db cluster update failed: %v", err)
					err := updateClusterStatus(context.Background(), newDBCluster, crd.DBClusterStatus{Message: fmt.Sprintf("%v", err), State: Failed}, _client)
					if err != nil {
						log.Printf("db cluster CRD status update failed: %v", err)
					}
				}
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
	go clusterController.Run(stop)
	// Wait forever
	select {}
}

func getProvider(provider string, publiclyAccessible bool, dbprovider, repository string) (provider.DatabaseProvider, error) {
	kubectl, err := getKubectl()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	_provider := dbprovider
	if provider != "" {
		_provider = provider
	}
	switch _provider {
	case "aws":
		r, err := rds.New(context.Background(), publiclyAccessible, kubectl)
		if err != nil {
			return nil, err
		}
		return r, nil

	case "local":
		r, err := local.New(kubectl, repository)
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
		err := updateStatus(context.Background(), db, crd.DatabaseStatus{Message: "Creating", State: "Creating"}, crdclient)
		if err != nil {
			return fmt.Errorf("database CRD status update failed: %v", err)
		}
	}

	log.Println("trying to get kubectl")

	r, err := getProvider(db.Spec.Provider, db.Spec.PubliclyAccessible, dbprovider, repository)
	if err != nil {
		return err
	}

	hostname, err := r.CreateDatabase(ctx, db)
	if err != nil {
		return err
	}

	log.Printf("Creating service '%v' for %v\n", db.Name, hostname)
	err = r.CreateService(ctx, db.Namespace, hostname, db.Name)
	if err != nil {
		return err
	}

	err = updateStatus(context.Background(), db, crd.DatabaseStatus{Message: "Created", State: "Created"}, crdclient)
	if err != nil {
		return err
	}
	log.Printf("Creation of database %v done\n", db.Name)
	return nil
}

func handleCreateDBCluster(ctx context.Context, cluster *crd.DBCluster, crdclient *client.DBClusterCrdclient, provider, repository string) error {
	if cluster.Status.State == "Created" && provider == "aws" {
		log.Printf("DB cluster %v already created, skipping\n", cluster.Name)
		return nil
	}

	if cluster.Status.State != "Created" {
		err := updateClusterStatus(context.Background(), cluster, crd.DBClusterStatus{Message: "Creating", State: "Creating"}, crdclient)
		if err != nil {
			return fmt.Errorf("DB cluster CRD status update failed: %v", err)
		}
	}

	log.Println("trying to get kubectl")
	publiclyAccessible := false
	if cluster.Spec.PubliclyAccessible != nil {
		publiclyAccessible = *cluster.Spec.PubliclyAccessible
	}
	r, err := getProvider(cluster.Spec.Provider, publiclyAccessible, provider, repository)
	if err != nil {
		return err
	}

	hostname, err := r.CreateDBCluster(ctx, cluster)
	if err != nil {
		return err
	}

	log.Printf("Creating service '%v' for %v\n", cluster.Name, hostname)
	err = r.CreateService(ctx, cluster.Namespace, hostname, cluster.Name)
	if err != nil {
		return err
	}

	err = updateClusterStatus(context.Background(), cluster, crd.DBClusterStatus{Message: "Created", State: "Created"}, crdclient)
	if err != nil {
		return err
	}
	log.Printf("Creation of DB cluster %v done\n", cluster.Name)
	return nil
}

func handleUpdateDBCluster(ctx context.Context, cluster *crd.DBCluster, crdclient *client.DBClusterCrdclient, dbprovider, repository string) error {
	publiclyAccessible := false
	if cluster.Spec.PubliclyAccessible != nil {
		publiclyAccessible = *cluster.Spec.PubliclyAccessible
	}
	r, err := getProvider(cluster.Spec.Provider, publiclyAccessible, dbprovider, repository)
	if err != nil {
		log.Printf("Updating db cluster: failed getting provider:%v\n", err)
		return err
	}

	err = r.UpdateDBCluster(ctx, cluster)
	if err != nil {
		log.Printf("Updating db cluster: UpdateDBCluster:%v\n", err)
		return err
	}

	err = updateClusterStatus(context.Background(), cluster, crd.DBClusterStatus{Message: "Updated", State: "Updated"}, crdclient)
	if err != nil {
		log.Printf("Updating db cluster: updateClusterStatus:%v\n", err)
		return err
	}
	log.Printf("Update of db cluster %v done\n", cluster.Name)
	return nil
}

func handleUpdateDatabase(ctx context.Context, db *crd.Database, crdclient *client.Crdclient, dbprovider, repository string) error {
	r, err := getProvider(db.Spec.Provider, db.Spec.PubliclyAccessible, dbprovider, repository)
	if err != nil {
		log.Printf("Updating database: failed getting provider:%v\n", err)
		return err
	}

	err = r.UpdateDatabase(ctx, db)
	if err != nil {
		log.Printf("Updating database: UpdateDatabase:%v\n", err)
		return err
	}

	err = updateStatus(context.Background(), db, crd.DatabaseStatus{Message: "Updated", State: "Updated"}, crdclient)
	if err != nil {
		log.Printf("Updating database: updateStatus:%v\n", err)
		return err
	}
	log.Printf("Update of database %v done\n", db.Name)
	return nil
}

func updateClusterStatus(ctx context.Context, cluster *crd.DBCluster, status crd.DBClusterStatus, crdclient *client.DBClusterCrdclient) error {
	cluster, err := crdclient.Get(ctx, cluster.Name)
	if err != nil {
		return err
	}

	cluster.Status = status
	_, err = crdclient.Update(ctx, cluster)
	if err != nil {
		return err
	}
	return nil
}

func updateStatus(ctx context.Context, db *crd.Database, status crd.DatabaseStatus, crdclient *client.Crdclient) error {
	db, err := crdclient.Get(ctx, db.Name)
	if err != nil {
		return err
	}

	db.Status = status
	_, err = crdclient.Update(ctx, db)
	if err != nil {
		return err
	}
	return nil
}

func excluded(namespace, name string, excludeNamespaces, includeNamespaces []string) bool {
	if len(excludeNamespaces) > 0 && stringInSlice(namespace, excludeNamespaces) {
		log.Printf("database %s is in excluded namespace %s. Ignoring...", name, namespace)
		return true
	}
	if len(includeNamespaces) > 0 && !stringInSlice(namespace, includeNamespaces) {
		log.Printf("database %s is in a non included namespace %s. Ignoring...", name, namespace)
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
