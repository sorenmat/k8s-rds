package main

import (
	"fmt"
	"log"
	"time"

	"github.com/sorenmat/cool-aide/crd"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/rds"
	_ "github.com/lib/pq"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
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
func getKubectl() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Println("Appears we are not running in a cluster")
		config, err = clientcmd.BuildConfigFromFlags("", home()+"/.kube/config")
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

func kubestuff() []*string {
	publiclyAccessible := false

	kubectl := getKubectl()

	nodes, err := kubectl.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		log.Fatal("unable to get nodes")
	}
	name := ""
	for _, n := range nodes.Items {
		name = n.Spec.ExternalID
	}
	log.Println("Found node with ID: ", name)
	creds := credentials.NewSharedCredentials("", "testing")
	cfg := &aws.Config{
		Credentials: creds,
		Region:      aws.String("eu-west-1"),
		CredentialsChainVerboseErrors: aws.Bool(true),
	}
	ses, err := session.NewSession()
	if err != nil {
		log.Fatal("unable to create session: ", err)
	}
	svc := ec2.New(ses, cfg)

	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
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
		subnets, err := svc.DescribeSubnets(&ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{&ec2.Filter{Name: aws.String("vpc-id"), Values: []*string{vpcID}}}})
		if err != nil {
			log.Fatal(err)
		}
		for _, v := range subnets.Subnets {
			if *v.MapPublicIpOnLaunch == publiclyAccessible {
				result = append(result, v.SubnetId)
			}
		}

	}
	log.Printf("Found the follwing subnets: ")
	for _, v := range result {
		log.Printf(*v + " ")
	}
	return result
}

func deleteDatabase(db crd.Database) {
}

func createDatabase(db crd.Database) {
	subnets := kubestuff()
	subnetDescription := "my database"
	subnetName := "mysubnet"
	creds := credentials.NewSharedCredentials("", "testing")
	cfg := &aws.Config{
		Credentials: creds,
		Region:      aws.String("eu-west-1"),
		CredentialsChainVerboseErrors: aws.Bool(true),
	}

	svc := rds.New(session.New(cfg))

	sf := &rds.DescribeDBSubnetGroupsInput{DBSubnetGroupName: aws.String(subnetName)}
	_, err := svc.DescribeDBSubnetGroups(sf)
	if err != nil {
		// assume we didn't find it..
		subnet := &rds.CreateDBSubnetGroupInput{
			DBSubnetGroupDescription: aws.String(subnetDescription),
			DBSubnetGroupName:        aws.String(subnetName),
			SubnetIds:                subnets,
			Tags:                     []*rds.Tag{&rds.Tag{Key: aws.String("DBName"), Value: aws.String(db.Spec.DBName)}},
		}
		_, err := svc.CreateDBSubnetGroup(subnet)
		if err != nil {
			log.Println(errors.Wrap(err, "CreateDBSubnetGroup"))
		}
	}

	input := &rds.CreateDBInstanceInput{
		AllocatedStorage:     aws.Int64(5),
		DBInstanceClass:      aws.String(db.Spec.Class),
		DBInstanceIdentifier: aws.String(db.Spec.DBName),
		Engine:               aws.String("postgres"),
		MasterUserPassword:   aws.String(db.Spec.Password),
		MasterUsername:       aws.String(db.Spec.Username),
		AvailabilityZone:     aws.String("eu-west-1a"),
		DBSubnetGroupName:    aws.String(subnetName),
		PubliclyAccessible:   aws.Bool(false),
	}
	_, err = svc.CreateDBInstance(input)
	if err != nil {
		log.Println(errors.Wrap(err, "CreateDBInstance"))
	}
	status := ""

	var rdsdb *rds.DBInstance
	for {
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
		result2, err := svc.DescribeDBInstances(k)
		if err != nil {
			log.Println(err)
			break
		}
		rdsdb = result2.DBInstances[0]
		status = *rdsdb.DBInstanceStatus
		if *rdsdb.DBInstanceStatus == "available" {
			break
		}
		log.Println("Wait for db to become ready, staus was", status)
		time.Sleep(3 * time.Second)
	}

	dbHostname := *rdsdb.Endpoint.Address

	kubectl := getKubectl()
	namespace := "default"
	serviceInterface := kubectl.CoreV1().Services(namespace)
	syncService(serviceInterface, "default", dbHostname, db.Spec.Name)

}

func FillService(s *v1.Service, namespace string, hostname string, internalname string) *v1.Service {
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
func syncService(serviceInterface corev1.ServiceInterface, namespace, hostname string, internalname string) {
	s, sErr := serviceInterface.Get(hostname, metav1.GetOptions{})

	if sErr == nil {
		log.Printf("Updating service for: %v\n", hostname)
		s = FillService(s, namespace, hostname, internalname)
		_, err := serviceInterface.Update(s)
		if err != nil {
			log.Println("Error updating service", err)
		}
	} else {
		log.Printf("Creating new service for: %v\n", hostname)
		k8sService := &v1.Service{}
		s = FillService(k8sService, namespace, hostname, internalname)
		_, err := serviceInterface.Create(s)
		if err != nil {
			log.Println("Error creating service", err)
		}
	}
}
