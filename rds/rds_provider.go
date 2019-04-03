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

// AWS ...
type AWS struct {
	RDS            *rds.RDS
	EC2            *ec2.EC2
	Subnets        []string
	SecurityGroups []string
}

// CreateDatabase ...
func (a *AWS) CreateDatabase(db *crd.Database, password string) (string, error) {
	log.Println("Trying to find the correct subnets")
	subnetName, err := a.ensureSubnets(db)
	if err != nil {
		return "", err
	}

	input := convertSpecToInputCreate(db, subnetName, a.SecurityGroups, password)

	// search for the instance
	log.Printf("Trying to find db instance %v\n", db.Spec.DBName)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
	res := a.RDS.DescribeDBInstancesRequest(k)
	_, err = res.Send()
	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		log.Printf("DB instance %v not found trying to create it\n", db.Spec.DBName)
		// seems like we didn't find a database with this name, let's create on
		res := a.RDS.CreateDBInstanceRequest(input)
		_, err = res.Send()
		if err != nil {
			return "", errors.Wrap(err, "CreateDBInstance")
		}

		log.Printf("Waiting for db instance %v to become available\n", input.DBInstanceIdentifier)
		time.Sleep(5 * time.Second)
		err = a.RDS.WaitUntilDBInstanceAvailable(k)
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", input.DBInstanceIdentifier))
		}

		log.Printf("Reboot instance after creation %v to apply params\n", *input.DBInstanceIdentifier)
		r := &rds.RebootDBInstanceInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
		_, err = a.RDS.RebootDBInstanceRequest(r).Send()
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("something went wrong in RebootDBInstanceRequest for db instance %v", input.DBInstanceIdentifier))
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", input.DBInstanceIdentifier))
	}

	log.Printf("Waiting for db instance %v to become available after create\n", *input.DBInstanceIdentifier)
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

// RestoreDatabase ...
func (a *AWS) RestoreDatabase(db *crd.Database) (string, error) {
	log.Println("Trying to find the correct subnets")
	subnetName, err := a.ensureSubnets(db)
	if err != nil {
		return "", err
	}

	var securityGroups []string
	if len(db.Spec.VpcSecurityGroupIds) > 0 {
		securityGroups = append(securityGroups, db.Spec.VpcSecurityGroupIds)
	} else {
		securityGroups = a.SecurityGroups
	}

	input := convertSpecToInputRestore(db, subnetName, securityGroups)

	fmt.Printf("%v\n", subnetName)
	fmt.Printf("%v\n", a.SecurityGroups)
	fmt.Printf("%v\n", input)
	//panic("Nope")

	// search for the instance
	log.Printf("Trying to find db instance %v\n", db.Spec.DBName)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
	res := a.RDS.DescribeDBInstancesRequest(k)
	_, err = res.Send()
	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		log.Printf("DB instance %v not found trying to create it\n", db.Spec.DBName)
		// seems like we didn't find a database with this name, let's create on
		res := a.RDS.RestoreDBInstanceFromDBSnapshotRequest(input)
		_, err = res.Send()
		if err != nil {
			return "", errors.Wrap(err, "RestoreDBInstance")
		}

		log.Printf("Waiting for db instance %v to become available after restoring\n", *input.DBInstanceIdentifier)
		time.Sleep(5 * time.Second)
		err = a.RDS.WaitUntilDBInstanceAvailable(k)
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", input.DBInstanceIdentifier))
		}

		log.Printf("Reboot instance after restoring %v to apply params\n", *input.DBInstanceIdentifier)
		r := &rds.RebootDBInstanceInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
		_, err = a.RDS.RebootDBInstanceRequest(r).Send()
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("something went wrong in RebootDBInstanceRequest for db instance %v", input.DBInstanceIdentifier))
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", input.DBInstanceIdentifier))
	}

	log.Printf("Waiting for db instance %v to become available after restore\n", *input.DBInstanceIdentifier)
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

// DeleteDatabase ...
func (a *AWS) DeleteDatabase(db *crd.Database) {
	// delete the database instance
	svc := a.RDS
	dbName := db.Name
	t := time.Now()
	finalSnapshotIdentifier := fmt.Sprintf("k8s-rds-%v-%v", dbName, t.Format("20060102150405"))

	log.Printf("DB: %v to be deleted, with finalSnapshot: %v\n", dbName, finalSnapshotIdentifier)
	res := svc.DeleteDBInstanceRequest(&rds.DeleteDBInstanceInput{
		DBInstanceIdentifier:      aws.String(dbName),
		FinalDBSnapshotIdentifier: aws.String(finalSnapshotIdentifier),
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

	// delete subnetgroup only for creation process
	//if db.Spec.DBSnapshotIdentifier == "" {
	//	log.Printf("SubnetGroup %v to be deleted\n", db.Spec.DBSubnetGroupName)
	//	a.deleteSubnetGroup(db)
	//}
}

// deleteSubnetGroup ...
func (a *AWS) deleteSubnetGroup(db *crd.Database) {
	svc := a.RDS
	// delete the subnet group attached to the instance
	subnetName := db.Spec.DBSubnetGroupName
	dres := svc.DeleteDBSubnetGroupRequest(&rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(subnetName)})
	_, err := dres.Send()
	if err != nil {
		log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete subnet %v", subnetName)))
	} else {
		log.Println("Deleted DBSubnet group: ", subnetName)
	}
}

func (a *AWS) ensureSubnets(db *crd.Database) (string, error) {
	if len(a.Subnets) == 0 {
		log.Println("Error: unable to continue due to lack of subnets, perhaps we couldn't lookup the subnets")
	}
	subnetDescription := "subnet k8s-rds"
	subnetName := db.Spec.DBSubnetGroupName

	svc := a.RDS

	sf := &rds.DescribeDBSubnetGroupsInput{DBSubnetGroupName: aws.String(subnetName)}
	res := svc.DescribeDBSubnetGroupsRequest(sf)
	_, err := res.Send()
	log.Println("Subnets:", a.Subnets)
	if err != nil {
		// assume we didn't find it..
		subnet := &rds.CreateDBSubnetGroupInput{
			DBSubnetGroupDescription: aws.String(subnetDescription),
			DBSubnetGroupName:        aws.String(subnetName),
			SubnetIds:                a.Subnets,
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

func convertSpecToInputRestore(v *crd.Database, subnetName string, securityGroups []string) *rds.RestoreDBInstanceFromDBSnapshotInput {
	return &rds.RestoreDBInstanceFromDBSnapshotInput{
		AvailabilityZone:     aws.String(v.Spec.AvailabilityZone),
		CopyTagsToSnapshot:   aws.Bool(v.Spec.CopyTagsToSnapshot),
		DBInstanceClass:      aws.String(v.Spec.Class),
		DBInstanceIdentifier: aws.String(v.Name),
		DBName:               aws.String(v.Spec.DBName),
		DBParameterGroupName: aws.String(v.Spec.DBParameterGroupName),
		DBSnapshotIdentifier: aws.String(v.Spec.DBSnapshotIdentifier),
		DBSubnetGroupName:    aws.String(subnetName),
		Engine:               aws.String(v.Spec.Engine),
		LicenseModel:         aws.String("license-included"),
		MultiAZ:              aws.Bool(v.Spec.MultiAZ),
		PubliclyAccessible:   aws.Bool(v.Spec.PubliclyAccessible),
		StorageType:          aws.String(v.Spec.StorageType),
		Tags:                 createTags(v.Spec.Tags),
		VpcSecurityGroupIds:  securityGroups,
	}
}

func convertSpecToInputCreate(v *crd.Database, subnetName string, securityGroups []string, password string) *rds.CreateDBInstanceInput {
	input := &rds.CreateDBInstanceInput{
		AllocatedStorage:      aws.Int64(v.Spec.Size),
		AvailabilityZone:      aws.String(v.Spec.AvailabilityZone),
		BackupRetentionPeriod: aws.Int64(v.Spec.BackupRetentionPeriod),
		DBInstanceClass:       aws.String(v.Spec.Class),
		DBInstanceIdentifier:  aws.String(v.Name),
		DBName:                aws.String(v.Spec.DBName),
		DBParameterGroupName:  aws.String(v.Spec.DBParameterGroupName),
		DBSubnetGroupName:     aws.String(subnetName),
		Engine:                aws.String(v.Spec.Engine),
		EngineVersion:         aws.String(v.Spec.EngineVersion),
		MasterUserPassword:    aws.String(password),
		MasterUsername:        aws.String(v.Spec.Username),
		MultiAZ:               aws.Bool(v.Spec.MultiAZ),
		PubliclyAccessible:    aws.Bool(v.Spec.PubliclyAccessible),
		StorageEncrypted:      aws.Bool(v.Spec.StorageEncrypted),
		Tags:                  createTags(v.Spec.Tags),
		VpcSecurityGroupIds:   securityGroups,
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int64(v.Spec.Iops)
	}
	return input
}

func createTags(t map[string]string) []rds.Tag {
	var tags []rds.Tag

	for k, v := range t {
		tags = append(tags, rds.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	return tags
}
