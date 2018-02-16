package main

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/rds"
	_ "github.com/lib/pq"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/client"
	"github.com/sorenmat/k8s-rds/crd"
	"k8s.io/api/core/v1"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
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

func getKubectl() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Println("Appears we are not running in a cluster")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig())
		if err != nil {
			panic(err.Error())
		}
	} else {
		log.Println("Seems like we are running in a Kubernetes cluster!!")
	}

	kubectl, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(errors.Wrap(err, "unable create kubectl"))
	}
	return kubectl
}

func ec2config() *aws.Config {
	return &aws.Config{
		Region: aws.String("eu-west-1"),
		CredentialsChainVerboseErrors: aws.Bool(true),
	}
}

func ec2client() *ec2.EC2 {
	ses, err := session.NewSession(ec2config())
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	svc := ec2.New(ses, ec2config())
	return svc
}

func rdsclient() *rds.RDS {
	return rds.New(session.New(ec2config()))
}

// getSubnets returns a list of subnets that the RDS instance should be attached to
// We do this by findind a node in the cluster, take the VPC id from that node a list
// the security groups in the VPC
func getSubnets(public bool) ([]*string, error) {
	kubectl := getKubectl()

	nodes, err := kubectl.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		log.Fatal("unable to get nodes")
	}
	name := ""
	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = nodes.Items[0].Spec.ExternalID
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Println("Found node with ID: ", name)

	svc := ec2client()

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
		log.Fatal(err)
	}
	var result []*string
	if len(res.Reservations) >= 1 {
		vpcID := res.Reservations[0].Instances[0].VpcId
		log.Printf("Found VPC %v will search for subnet in that VPC\n", *vpcID)
		subnets, err := svc.DescribeSubnets(&ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{{Name: aws.String("vpc-id"), Values: []*string{vpcID}}}})
		if err != nil {
			log.Fatal(err)
		}
		for _, v := range subnets.Subnets {
			if *v.MapPublicIpOnLaunch == public {
				result = append(result, v.SubnetId)
			}
		}

	}
	log.Printf("Found the follwing subnets: ")
	for _, v := range result {
		log.Printf(*v + " ")
	}
	return result, nil
}

func deleteDatabase(db *crd.Database) {

	// delete the service first, this way we can't get more traffic to the instance
	serviceInterface := getKubectl().CoreV1().Services(db.Namespace)
	err := serviceInterface.Delete(db.Spec.Name, &metav1.DeleteOptions{})
	if err != nil {
		log.Println(err)
	}

	// delete the database instance
	svc := rdsclient()
	_, err = svc.DeleteDBInstance(&rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(db.Spec.DBName),
		SkipFinalSnapshot:    aws.Bool(true),
	})
	if err != nil {
		log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete database %v", db.Spec.DBName)))
	} else {
		waitForDBState(svc, db, "deleted")
		log.Println("Deleted DB instance: ", db.Spec.DBName)
	}

	// delete the subnet group attached to the instance
	subnetName := db.Name + "-subnet"
	_, err = svc.DeleteDBSubnetGroup(&rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(subnetName)})
	if err != nil {
		log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete subnet %v", subnetName)))
	} else {
		log.Println("Deleted DBSubnet group: ", subnetName)
	}

}

func convertSpecToInput(v *crd.Database, subnetName string) *rds.CreateDBInstanceInput {
	input := &rds.CreateDBInstanceInput{
		AllocatedStorage:      aws.Int64(v.Spec.Size),
		DBInstanceClass:       aws.String(v.Spec.Class),
		DBInstanceIdentifier:  aws.String(v.Spec.DBName),
		Engine:                aws.String(v.Spec.Engine),
		MasterUserPassword:    aws.String(v.Spec.Password),
		MasterUsername:        aws.String(v.Spec.Username),
		DBSubnetGroupName:     aws.String(subnetName),
		PubliclyAccessible:    aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:               aws.Bool(v.Spec.MultiAZ),
		StorageEncrypted:      aws.Bool(v.Spec.StorageEncrypted),
		BackupRetentionPeriod: aws.Int64(0), //disable backups
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int64(v.Spec.Iops)
	}
	return input
}

func waitForDBState(svc *rds.RDS, db *crd.Database, status string) {
	var rdsdb *rds.DBInstance
	for {
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
		result2, err := svc.DescribeDBInstances(k)
		if err != nil {
			log.Println(err)
			break
		}
		rdsdb = result2.DBInstances[0]

		if *rdsdb.DBInstanceStatus == status {
			break
		}
		log.Printf("Wait for db status to be %v was %v\n", status, *rdsdb.DBInstanceStatus)
		time.Sleep(5 * time.Second)
	}
}

func createDatabase(db *crd.Database, crdclient *client.Crdclient) error {
	subnets, err := getSubnets(db.Spec.PubliclyAccessible)
	if err != nil {
		return err
	}
	subnetDescription := "subnet for " + db.Name + " in namespace " + db.Namespace
	subnetName := db.Name + "-subnet"

	svc := rdsclient()

	sf := &rds.DescribeDBSubnetGroupsInput{DBSubnetGroupName: aws.String(subnetName)}
	_, err = svc.DescribeDBSubnetGroups(sf)
	if err != nil {
		// assume we didn't find it..
		subnet := &rds.CreateDBSubnetGroupInput{
			DBSubnetGroupDescription: aws.String(subnetDescription),
			DBSubnetGroupName:        aws.String(subnetName),
			SubnetIds:                subnets,
			Tags:                     []*rds.Tag{{Key: aws.String("DBName"), Value: aws.String(db.Spec.DBName)}},
		}
		_, err := svc.CreateDBSubnetGroup(subnet)
		if err != nil {
			log.Println(errors.Wrap(err, "CreateDBSubnetGroup"))
		}
	}
	input := convertSpecToInput(db, subnetName)

	// search for the instance
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
	result2, err := svc.DescribeDBInstances(k)
	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		// seems like we didn't find a database with this name, let's create on
		_, err = svc.CreateDBInstance(input)
		if err != nil {
			return (errors.Wrap(err, "CreateDBInstance"))
		}
	} else if err != nil {
		return errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", db.Spec.DBName))
	}
	var rdsdb *rds.DBInstance
	waitForDBState(svc, db, "available")

	// Get the newly created database so we can get the endpoint
	k = &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
	result2, err = svc.DescribeDBInstances(k)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", db.Spec.DBName))
	}
	rdsdb = result2.DBInstances[0]

	dbHostname := *rdsdb.Endpoint.Address

	kubectl := getKubectl()
	// create a service in kubernetes that points to the AWS RDS instance
	serviceInterface := kubectl.CoreV1().Services(db.Namespace)
	err = syncService(serviceInterface, db.Namespace, dbHostname, db.Spec.Name)
	return err
}

// create an External nameed service object for Kubernetes
func createService(s *v1.Service, namespace string, hostname string, internalname string) *v1.Service {
	var ports []v1.ServicePort

	ports = append(ports, v1.ServicePort{
		Name:       fmt.Sprintf("pgsql"),
		Port:       int32(5432),
		TargetPort: intstr.IntOrString{IntVal: int32(5432)},
	})
	s.Spec.Type = "ExternalName"
	s.Spec.ExternalName = hostname

	s.Spec.Ports = ports
	s.Name = internalname
	s.Annotations = map[string]string{"origin": "rds"}
	s.Namespace = namespace
	return s
}

// syncService Update the service in Kubernetes with the new information
func syncService(serviceInterface corev1.ServiceInterface, namespace, hostname string, internalname string) error {
	s, sErr := serviceInterface.Get(hostname, metav1.GetOptions{})

	if sErr != nil {
		s = &v1.Service{}
	}
	s = createService(s, namespace, hostname, internalname)
	s, err := serviceInterface.Update(s)
	return err
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

				db.Status = crd.DatabaseStatus{Message: "Creating", State: "Creating"}
				_, err = crdclient.Update(db)
				if err == nil {
					log.Println("Database CRD updated")
				} else {
					log.Println("Database CRD update failed: ", err)
				}
				err := createDatabase(db, crdclient)
				if err != nil {
					log.Println(err)
				}
			},
			DeleteFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				log.Printf("deleting database: %s \n", db.Name)
				deleteDatabase(db)
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
