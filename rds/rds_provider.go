package rds

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/crd"
	"github.com/sorenmat/k8s-rds/provider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type RDS struct {
	EC2             *ec2.EC2
	Subnets         []string
	SecurityGroups  []string
	ServiceProvider provider.ServiceProvider
}

func New(db *crd.Database, kc *kubernetes.Clientset) (*RDS, error) {
	ec2client, err := ec2client(kc)
	if err != nil {
		log.Fatal("unable to create a client for EC2 ", err)
	}

	log.Println("trying to get subnets")
	subnets, err := getSubnets(kc, ec2client, db.Spec.PubliclyAccessible)
	if err != nil {
		return nil, fmt.Errorf("unable to get subnets from instance: %v", err)

	}
	log.Println("trying to get security groups")
	sgs, err := getSGS(kc, ec2client)
	if err != nil {
		return nil, fmt.Errorf("unable to get security groups from instance: %v", err)

	}

	r := RDS{EC2: ec2client, Subnets: subnets, SecurityGroups: sgs}
	return &r, nil
}

// CreateDatabase creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) CreateDatabase(db *crd.Database) (string, error) {
	// Ensure that the subnets for the DB is create or updated
	log.Println("Trying to find the correct subnets")
	subnetName, err := r.ensureSubnets(db)
	if err != nil {
		return "", err
	}

	log.Printf("getting secret: Name: %v Key: %v \n", db.Spec.Password.Name, db.Spec.Password.Key)
	pw, err := r.ServiceProvider.GetSecret(db.Namespace, db.Spec.Password.Name, db.Spec.Password.Key)
	if err != nil {
		return "", err
	}
	input := convertSpecToInput(db, subnetName, r.SecurityGroups, pw)

	// search for the instance
	log.Printf("Trying to find db instance %v\n", db.Spec.DBName)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
	res := r.rdsclient().DescribeDBInstancesRequest(k)
	_, err = res.Send()
	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		log.Printf("DB instance %v not found trying to create it\n", db.Spec.DBName)
		// seems like we didn't find a database with this name, let's create on
		res := r.rdsclient().CreateDBInstanceRequest(input)
		_, err = res.Send()
		if err != nil {
			return "", errors.Wrap(err, "CreateDBInstance")
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", input.DBInstanceIdentifier))
	}
	log.Printf("Waiting for db instance %v to become available\n", input.DBInstanceIdentifier)
	time.Sleep(5 * time.Second)
	err = r.rdsclient().WaitUntilDBInstanceAvailable(k)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", input.DBInstanceIdentifier))
	}

	// Get the newly created database so we can get the endpoint
	dbHostname, err := getEndpoint(input.DBInstanceIdentifier, r.rdsclient())
	if err != nil {
		return "", err
	}
	return dbHostname, nil
}

// ensureSubnets is ensuring that we have created or updated the subnet according to the data from the CRD object
func (r *RDS) ensureSubnets(db *crd.Database) (string, error) {
	if len(r.Subnets) == 0 {
		log.Println("Error: unable to continue due to lack of subnets, perhaps we couldn't lookup the subnets")
	}
	subnetDescription := "subnet for " + db.Name + " in namespace " + db.Namespace
	subnetName := db.Name + "-subnet-" + db.Namespace

	svc := r.rdsclient()

	sf := &rds.DescribeDBSubnetGroupsInput{DBSubnetGroupName: aws.String(subnetName)}
	res := svc.DescribeDBSubnetGroupsRequest(sf)
	_, err := res.Send()
	log.Println("Subnets:", r.Subnets)
	if err != nil {
		// assume we didn't find it..
		subnet := &rds.CreateDBSubnetGroupInput{
			DBSubnetGroupDescription: aws.String(subnetDescription),
			DBSubnetGroupName:        aws.String(subnetName),
			SubnetIds:                r.Subnets,
			Tags:                     []rds.Tag{{Key: aws.String("DBName"), Value: aws.String(db.Spec.DBName)}},
		}
		res := svc.CreateDBSubnetGroupRequest(subnet)
		_, err := res.Send()
		if err != nil {
			return "", errors.Wrap(err, "CreateDBSubnetGroup")
		}
	} else {
		log.Printf("Moving on seems like %v exsits", subnetName)
	}
	return subnetName, nil
}

func getEndpoint(dbName *string, svc *rds.RDS) (string, error) {
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: dbName}
	res := svc.DescribeDBInstancesRequest(k)
	instance, err := res.Send()
	if err != nil || len(instance.DBInstances) == 0 {
		return "", fmt.Errorf("wasn't able to describe the db instance with id %v", dbName)
	}
	rdsdb := instance.DBInstances[0]

	dbHostname := *rdsdb.Endpoint.Address
	return dbHostname, nil
}

func (r *RDS) DeleteDatabase(db *crd.Database) error {
	// delete the database instance
	svc := r.rdsclient()
	res := svc.DeleteDBInstanceRequest(&rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(db.Spec.DBName),
		SkipFinalSnapshot:    aws.Bool(true),
	})
	_, err := res.Send()
	if err != nil {
		e := errors.Wrap(err, fmt.Sprintf("unable to delete database %v", db.Spec.DBName))
		log.Println(e)
		return e
	} else {
		log.Printf("Waiting for db instance %v to be deleted\n", db.Spec.DBName)
		time.Sleep(5 * time.Second)
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
		err = r.rdsclient().WaitUntilDBInstanceDeleted(k)
		if err != nil {
			log.Println(err)
			return err
		} else {
			log.Println("Deleted DB instance: ", db.Spec.DBName)
		}
	}

	// delete the subnet group attached to the instance
	subnetName := db.Name + "-subnet"
	dres := svc.DeleteDBSubnetGroupRequest(&rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(subnetName)})
	_, err = dres.Send()
	if err != nil {
		e := errors.Wrap(err, fmt.Sprintf("unable to delete subnet %v", subnetName))
		log.Println(e)
		return e
	} else {
		log.Println("Deleted DBSubnet group: ", subnetName)
	}
	return nil
}

func (r *RDS) rdsclient() *rds.RDS {
	return rds.New(r.EC2.Config)
}

func convertSpecToInput(v *crd.Database, subnetName string, securityGroups []string, password string) *rds.CreateDBInstanceInput {
	input := &rds.CreateDBInstanceInput{
		DBName:                aws.String(v.Spec.DBName),
		AllocatedStorage:      aws.Int64(v.Spec.Size),
		DBInstanceClass:       aws.String(v.Spec.Class),
		DBInstanceIdentifier:  aws.String(v.Name + "-" + v.Namespace),
		VpcSecurityGroupIds:   securityGroups,
		Engine:                aws.String(v.Spec.Engine),
		MasterUserPassword:    aws.String(password),
		MasterUsername:        aws.String(v.Spec.Username),
		DBSubnetGroupName:     aws.String(subnetName),
		PubliclyAccessible:    aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:               aws.Bool(v.Spec.MultiAZ),
		StorageEncrypted:      aws.Bool(v.Spec.StorageEncrypted),
		BackupRetentionPeriod: aws.Int64(v.Spec.BackupRetentionPeriod),
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int64(v.Spec.Iops)
	}
	return input
}

// getSubnets returns a list of subnets that the RDS instance should be attached to
// We do this by finding a node in the cluster, take the VPC id from that node a list
// the security groups in the VPC
func getSubnets(kubectl *kubernetes.Clientset, svc *ec2.EC2, public bool) ([]string, error) {

	nodes, err := kubectl.CoreV1().Nodes().List(metav1.ListOptions{})
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

func getSGS(kubectl *kubernetes.Clientset, svc *ec2.EC2) ([]string, error) {

	nodes, err := kubectl.CoreV1().Nodes().List(metav1.ListOptions{})
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

func ec2client(kubectl *kubernetes.Clientset) (*ec2.EC2, error) {

	nodes, err := kubectl.CoreV1().Nodes().List(metav1.ListOptions{})
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
