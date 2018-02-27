package rds

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
)

type RDS struct {
	EC2config *aws.Config
	Subnets   []*string
}

// CreateDatabase creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) CreateDatabase(db *crd.Database, client *client.Crdclient, password string) (string, error) {
	// Ensure that the subnets for the DB is create or updated
	subnetName := r.ensureSubnets(db)

	input, err := convertSpecToInput(db, subnetName, password)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to get the secret for db %v", db.Spec.DBName))
	}

	// search for the instance
	log.Printf("Trying to find db instance %v\n", db.Spec.DBName)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
	_, err = r.rdsclient().DescribeDBInstances(k)
	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		log.Printf("DB instance %v not found trying to create it\n", db.Spec.DBName)
		// seems like we didn't find a database with this name, let's create on
		_, err = r.rdsclient().CreateDBInstance(input)
		if err != nil {
			return "", (errors.Wrap(err, "CreateDBInstance"))
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", db.Spec.DBName))
	}
	log.Printf("Waiting for db instance %v to become available\n", db.Spec.DBName)
	time.Sleep(5 * time.Second)
	err = r.rdsclient().WaitUntilDBInstanceAvailable(k)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", db.Spec.DBName))
	}
	//waitForDBState(r.rdsclient(), db, "available")
	// enable backup
	mod := &rds.ModifyDBInstanceInput{DBInstanceIdentifier: aws.String(db.Spec.DBName), BackupRetentionPeriod: aws.Int64(1)}
	_, err = r.rdsclient().ModifyDBInstance(mod)
	if err != nil {
		return "", (errors.Wrap(err, "enable backup"))
	}

	// Get the newly created database so we can get the endpoint
	dbHostname, err := getEndpoint(db.Spec.DBName, r.rdsclient())
	if err != nil {
		return "", err
	}
	return dbHostname, nil
}

// ensureSubnets is ensuring that we have created or updated the subnet according to the data from the CRD object
func (r *RDS) ensureSubnets(db *crd.Database) string {
	subnetDescription := "subnet for " + db.Name + " in namespace " + db.Namespace
	subnetName := db.Name + "-subnet"

	svc := r.rdsclient()

	sf := &rds.DescribeDBSubnetGroupsInput{DBSubnetGroupName: aws.String(subnetName)}
	_, err := svc.DescribeDBSubnetGroups(sf)
	if err != nil {
		// assume we didn't find it..
		subnet := &rds.CreateDBSubnetGroupInput{
			DBSubnetGroupDescription: aws.String(subnetDescription),
			DBSubnetGroupName:        aws.String(subnetName),
			SubnetIds:                r.Subnets,
			Tags:                     []*rds.Tag{{Key: aws.String("DBName"), Value: aws.String(db.Spec.DBName)}},
		}
		_, err := svc.CreateDBSubnetGroup(subnet)
		if err != nil {
			log.Println(errors.Wrap(err, "CreateDBSubnetGroup"))
		}
	}
	return subnetName
}

func getEndpoint(dbName string, svc *rds.RDS) (string, error) {
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(dbName)}
	instance, err := svc.DescribeDBInstances(k)
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
	_, err := svc.DeleteDBInstance(&rds.DeleteDBInstanceInput{
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

func (r *RDS) rdsclient() *rds.RDS {
	return rds.New(session.New(r.EC2config))
}

// waitForDBState wait for the RDS resource to reach a certain state
// This will check every 5 seconds
func waitForDBState(svc *rds.RDS, db *crd.Database, state string) error {
	var rdsdb *rds.DBInstance
	start := time.Now()
	for {
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(db.Spec.DBName)}
		result2, err := svc.DescribeDBInstances(k)
		if err != nil {
			log.Println(err)
			return errors.Wrap(err, fmt.Sprintf("waitForDBState could not describe the db instance %v", db.Spec.DBName))
		}
		rdsdb = result2.DBInstances[0]

		if *rdsdb.DBInstanceStatus == state {
			break
		}
		log.Printf("Wait for db status to be %v was %v\n", state, *rdsdb.DBInstanceStatus)
		time.Sleep(5 * time.Second)
	}
	stop := time.Now()
	log.Printf("Wait for change took: %v sec\n", (stop.Unix() - start.Unix()))
	return nil
}

func convertSpecToInput(v *crd.Database, subnetName string, password string) (*rds.CreateDBInstanceInput, error) {
	input := &rds.CreateDBInstanceInput{
		DBName:                aws.String(v.Spec.DBName),
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
