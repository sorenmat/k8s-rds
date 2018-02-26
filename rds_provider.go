package main

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/client"
	"github.com/sorenmat/k8s-rds/crd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RDS struct{}

func (r *RDS) CreateDatabase(db *crd.Database, client *client.Crdclient) error {
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
	input, err := convertSpecToInput(db, subnetName)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("wasn't able to get the secret for db %v", db.Spec.DBName))
	}

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
	err = syncService(serviceInterface, db.Namespace, dbHostname, db.Name)
	return err

}

func (r *RDS) DeleteDatabase(db *crd.Database) {

	// delete the service first, this way we can't get more traffic to the instance
	serviceInterface := getKubectl().CoreV1().Services(db.Namespace)
	err := serviceInterface.Delete(db.Name, &metav1.DeleteOptions{})
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

func rdsclient() *rds.RDS {
	return rds.New(session.New(ec2config()))
}

// waitForDBState wait for the RDS resource to reach a certain state
// This will check every 5 seconds
func waitForDBState(svc *rds.RDS, db *crd.Database, state string) error {
	var rdsdb *rds.DBInstance
	for {
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
		result2, err := svc.DescribeDBInstances(k)
		if err != nil {
			log.Println(err)
			return errors.Wrap(err, fmt.Sprintf("waitForDBState could not describe the db instance %v", db.Spec.DBName))
		}
		rdsdb = result2.DBInstances[0]

		if *rdsdb.DBInstanceStatus == state {
			return nil
		}
		log.Printf("Wait for db status to be %v was %v\n", state, *rdsdb.DBInstanceStatus)
		time.Sleep(5 * time.Second)
	}
}

func convertSpecToInput(v *crd.Database, subnetName string) (*rds.CreateDBInstanceInput, error) {
	secret, err := getKubectl().CoreV1().Secrets(v.Namespace).Get(v.Spec.Password.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to fetch secret %v", v.Spec.Password.Name))
	}
	password := secret.Data[v.Spec.Password.Key]
	input := &rds.CreateDBInstanceInput{
		AllocatedStorage:      aws.Int64(v.Spec.Size),
		DBInstanceClass:       aws.String(v.Spec.Class),
		DBInstanceIdentifier:  aws.String(v.Spec.DBName),
		Engine:                aws.String(v.Spec.Engine),
		MasterUserPassword:    aws.String(string(password)),
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
	return input, nil
}
