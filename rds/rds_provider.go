package rds

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/cloud104/k8s-rds/crd"
	"github.com/pkg/errors"
)

type RDS struct {
	EC2            *ec2.EC2
	Subnets        []string
	SecurityGroups []string
}

// CreateDatabase creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) CreateDatabase(db *crd.Database, password string) (string, error) {
	// Ensure that the subnets for the DB is create or updated
	log.Println("Trying to find the correct subnets")
	subnetName, err := r.ensureSubnets(db)
	if err != nil {
		return "", err
	}

	input := convertSpecToInput(db, subnetName, r.SecurityGroups, password)

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

func (r *RDS) DeleteDatabase(db *crd.Database) {
	// delete the database instance
	svc := r.rdsclient()
	res := svc.DeleteDBInstanceRequest(&rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(db.Spec.DBName),
		SkipFinalSnapshot:    aws.Bool(true),
	})
	_, err := res.Send()
	if err != nil {
		log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete database %v", db.Spec.DBName)))
	} else {
		log.Printf("Waiting for db instance %v to be deleted\n", db.Spec.DBName)
		time.Sleep(5 * time.Second)
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
		err = r.rdsclient().WaitUntilDBInstanceDeleted(k)
		if err != nil {
			log.Println(err)
		} else {
			log.Println("Deleted DB instance: ", db.Spec.DBName)
		}
	}

	// delete the subnet group attached to the instance
	subnetName := db.Name + "-subnet"
	dres := svc.DeleteDBSubnetGroupRequest(&rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(subnetName)})
	_, err = dres.Send()
	if err != nil {
		log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete subnet %v", subnetName)))
	} else {
		log.Println("Deleted DBSubnet group: ", subnetName)
	}
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
