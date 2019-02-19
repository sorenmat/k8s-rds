package rds

import (
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/cloud104/k8s-rds/crd"
	"github.com/pkg/errors"
	"log"
	"time"
)

type AWS struct {
	RDS *rds.RDS
}

func (a *AWS) RestoreDatabase(db *crd.Database) (string, error) {
	svc := a.RDS
	input := convertSpecToInputRestore(db)

	// search for the instance
	log.Printf("Trying to find db instance %v\n", db.Spec.DBName)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
	res := a.RDS.DescribeDBInstancesRequest(k)
	_, err := res.Send()

	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		log.Printf("DB instance %v not found trying to create it\n", db.Spec.DBName)
		// seems like we didn't find a database with this name, let's create on
		res := svc.RestoreDBInstanceFromDBSnapshotRequest(input)
		_, err = res.Send()
		if err != nil {
			return "", errors.Wrap(err, "CreateDBInstance")
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", input.DBInstanceIdentifier))
	}
	log.Printf("Waiting for db instance %v to become available\n", *input.DBInstanceIdentifier)
	time.Sleep(5 * time.Second)
	err = a.RDS.WaitUntilDBInstanceAvailable(k)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", input.DBInstanceIdentifier))
	}

	// Get the newly created database so we can get the endpoint
	dbHostname, err := getEndpoint(input.DBInstanceIdentifier, a.RDS)
	if err != nil {
		return "", err
	}
	return dbHostname, nil
}

func (a *AWS) DeleteDatabase(db *crd.Database) {
	// delete the database instance
	svc := a.RDS
	dbName := db.Spec.DBInstanceIdentifier
	log.Printf("DBName %v to be deleted\n", dbName)
	res := svc.DeleteDBInstanceRequest(&rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(dbName),
		// TODO production
		SkipFinalSnapshot: aws.Bool(true),
	})
	_, err := res.Send()
	if err != nil {
		log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete database %v", dbName)))
	} else {
		log.Printf("Waiting for db instance %v to be deleted\n", dbName)
		time.Sleep(5 * time.Second)
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(dbName)}
		err = svc.WaitUntilDBInstanceDeleted(k)
		if err != nil {
			log.Println(err)
		} else {
			log.Println("Deleted DB instance: ", dbName)
		}
	}
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

func convertSpecToInputRestore(v *crd.Database) *rds.RestoreDBInstanceFromDBSnapshotInput {
	var tags []rds.Tag

	input := &rds.RestoreDBInstanceFromDBSnapshotInput{
		Tags:                 tags,
		StorageType:          aws.String(v.Spec.StorageType),
		PubliclyAccessible:   aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:              aws.Bool(v.Spec.MultiAZ),
		Engine:               aws.String(v.Spec.Engine),
		DBSubnetGroupName:    aws.String(v.Spec.DBSubnetGroupName),
		DBName:               aws.String(v.Spec.DBName),
		DBInstanceIdentifier: aws.String(v.Spec.DBInstanceIdentifier),
		DBInstanceClass:      aws.String(v.Spec.Class),
		CopyTagsToSnapshot:   aws.Bool(v.Spec.CopyTagsToSnapshot),
	}

	input.LicenseModel = aws.String("license-included")
	input.DBSnapshotIdentifier = aws.String("arn:aws:rds:us-east-2:911270218041:snapshot:database-matriz-v26")
	input.AvailabilityZone = aws.String("us-east-2a")

	return input
}
