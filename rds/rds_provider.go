package rds

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/crd"
)

type RDS struct {
	EC2            *ec2.EC2
	Subnets        []string
	SecurityGroups []string
}

// CreateDatabase creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) CreateDatabase(db *crd.Database, password string) (string, error) {
	if db.Spec.DBSnapshotIdentifier == "" {
		return r.CreateDatabaseInstance(db, password)
	} else {
		return r.RestoreDatabaseFromSnapshot(db, password)
	}
}

// CreateDatabaseInstance creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) CreateDatabaseInstance(db *crd.Database, password string) (string, error) {
	var err error
	dbSubnetGroupName := db.Spec.DBSubnetGroupName
	if dbSubnetGroupName == "" {
		// Ensure that the subnets for the DB is create or updated
		log.Println("Trying to find the correct subnets")
		dbSubnetGroupName, err = r.ensureSubnets(db)
		if err != nil {
			return "", err
		}
	}

	input := convertSpecToInstanceInput(db, dbSubnetGroupName, r.SecurityGroups, password)

	// search for the instance
	log.Printf("Trying to find db instance %v\n", *input.DBInstanceIdentifier)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
	res := r.rdsclient().DescribeDBInstancesRequest(k)
	_, err = res.Send()
	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		log.Printf("DB instance %v not found trying to create it\n", *input.DBInstanceIdentifier)
		// seems like we didn't find a database with this name, let's create on
		res := r.rdsclient().CreateDBInstanceRequest(input)
		_, err = res.Send()
		if err != nil {
			return "", errors.Wrap(err, "CreateDBInstance")
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", *input.DBInstanceIdentifier))
	}
	log.Printf("Waiting for db instance %v to become available\n", *input.DBInstanceIdentifier)
	time.Sleep(5 * time.Second)
	err = r.rdsclient().WaitUntilDBInstanceAvailable(k)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", *input.DBInstanceIdentifier))
	}

	// Get the newly created database so we can get the endpoint
	dbHostname, err := getEndpoint(input.DBInstanceIdentifier, r.rdsclient())
	if err != nil {
		return "", err
	}
	return dbHostname, nil
}

// RestoreDatabaseFromSnapshot creates a database instance from a snapshot using the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) RestoreDatabaseFromSnapshot(db *crd.Database, password string) (string, error) {
	var err error
	dbSubnetGroupName := db.Spec.DBSubnetGroupName
	if dbSubnetGroupName == "" {
		// Ensure that the subnets for the DB is create or updated
		log.Println("Trying to find the correct subnets")
		dbSubnetGroupName, err = r.ensureSubnets(db)
		if err != nil {
			return "", err
		}
	}

	restoreSnapshotInput, modifyInstanceInput := convertSpecToRestoreSnapshotInput(db, dbSubnetGroupName, r.SecurityGroups, password)

	// search for the instance
	log.Printf("Trying to find db instance %v\n", *restoreSnapshotInput.DBInstanceIdentifier)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: restoreSnapshotInput.DBInstanceIdentifier}
	res := r.rdsclient().DescribeDBInstancesRequest(k)
	_, err = res.Send()
	if err != nil && err.Error() != rds.ErrCodeDBInstanceNotFoundFault {
		log.Printf("DB instance %v not found trying to restore it\n", *restoreSnapshotInput.DBInstanceIdentifier)
		// seems like we didn't find a database with this name, let's create on
		res := r.rdsclient().RestoreDBInstanceFromDBSnapshotRequest(restoreSnapshotInput)
		_, err = res.Send()
		if err != nil {
			return "", errors.Wrap(err, "RestoreDBInstanceFromDBSnapshot")
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", restoreSnapshotInput.DBInstanceIdentifier))
	} else {
		return "", errors.New(fmt.Sprintf("DB instance %v already exists. Will not restore", *restoreSnapshotInput.DBInstanceIdentifier))
	}
	log.Printf("Waiting for db instance %v to become available\n", *restoreSnapshotInput.DBInstanceIdentifier)
	time.Sleep(5 * time.Second)
	err = r.rdsclient().WaitUntilDBInstanceAvailable(k)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", *restoreSnapshotInput.DBInstanceIdentifier))
	}

	if modifyInstanceInput == nil {
		log.Printf("DB instance %v restored.\n", *restoreSnapshotInput.DBInstanceIdentifier)
	} else {
		// apply needed modifications
		log.Printf("DB instance %v restored. Applying some modifications\n", *restoreSnapshotInput.DBInstanceIdentifier)
		resModify := r.rdsclient().ModifyDBInstanceRequest(modifyInstanceInput)
		_, err = resModify.Send()
		if err != nil {
			return "", errors.Wrap(err, "ModifyDBInstance")
		}
		log.Printf("Waiting for db instance %v to become available\n", *restoreSnapshotInput.DBInstanceIdentifier)
		time.Sleep(5 * time.Second)
		err = r.rdsclient().WaitUntilDBInstanceAvailable(k)
		if err != nil {
			return "", errors.Wrap(err, fmt.Sprintf("something went wrong in WaitUntilDBInstanceAvailable for db instance %v", *restoreSnapshotInput.DBInstanceIdentifier))
		}
	}

	// Get the newly created database so we can get the endpoint
	dbHostname, err := getEndpoint(restoreSnapshotInput.DBInstanceIdentifier, r.rdsclient())
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
	instanceIdentifier := db.Name + "-" + db.Namespace
	svc := r.rdsclient()
	res := svc.DeleteDBInstanceRequest(&rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(instanceIdentifier),
		SkipFinalSnapshot:    aws.Bool(true),
	})
	_, err := res.Send()
	if err != nil {
		log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete database %v", instanceIdentifier)))
	} else {
		log.Printf("Waiting for db instance %v to be deleted\n", instanceIdentifier)
		time.Sleep(5 * time.Second)
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(instanceIdentifier)}
		err = r.rdsclient().WaitUntilDBInstanceDeleted(k)
		if err != nil {
			log.Println(err)
		} else {
			log.Println("Deleted DB instance: ", instanceIdentifier)
		}
	}

	// delete the subnet group attached to the instance if created previously
	dbSubnetGroupName := db.Spec.DBSubnetGroupName
	if dbSubnetGroupName == "" {
		dbSubnetGroupName := db.Name + "-subnet-" + db.Namespace
		dres := svc.DeleteDBSubnetGroupRequest(&rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(dbSubnetGroupName)})
		_, err = dres.Send()
		if err != nil {
			log.Println(errors.Wrap(err, fmt.Sprintf("unable to delete subnet %v", dbSubnetGroupName)))
		} else {
			log.Println("Deleted DBSubnet group: ", dbSubnetGroupName)
		}
	}
}

func (r *RDS) rdsclient() *rds.RDS {
	return rds.New(r.EC2.Config)
}

func convertSpecToInstanceInput(v *crd.Database, dbSubnetGroupName string, securityGroups []string, password string) *rds.CreateDBInstanceInput {
	input := &rds.CreateDBInstanceInput{
		DBName:               aws.String(v.Spec.DBName),
		AllocatedStorage:     aws.Int64(v.Spec.Size),
		DBInstanceClass:      aws.String(v.Spec.Class),
		DBInstanceIdentifier: aws.String(v.Name + "-" + v.Namespace),
		VpcSecurityGroupIds:  securityGroups,
		Engine:               aws.String(v.Spec.Engine),
		MasterUserPassword:   aws.String(password),
		MasterUsername:       aws.String(v.Spec.Username),
		DBSubnetGroupName:    aws.String(dbSubnetGroupName),
		PubliclyAccessible:   aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:              aws.Bool(v.Spec.MultiAZ),
		StorageEncrypted:     aws.Bool(v.Spec.StorageEncrypted),
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int64(v.Spec.Iops)
	}
	if v.Spec.DBParameterGroupName != "" {
		input.DBParameterGroupName = aws.String(v.Spec.DBParameterGroupName)
	}
	if v.Spec.BackupRetentionPeriod != nil {
		input.BackupRetentionPeriod = aws.Int64(*v.Spec.BackupRetentionPeriod)
	}
	return input
}

func convertSpecToRestoreSnapshotInput(v *crd.Database, dbSubnetGroupName string, securityGroups []string, password string) (*rds.RestoreDBInstanceFromDBSnapshotInput, *rds.ModifyDBInstanceInput) {
	restoreSnapshotInput := &rds.RestoreDBInstanceFromDBSnapshotInput{
		DBName:               aws.String(v.Spec.DBName),
		DBInstanceClass:      aws.String(v.Spec.Class),
		DBInstanceIdentifier: aws.String(v.Name + "-" + v.Namespace),
		DBSnapshotIdentifier: aws.String(v.Spec.DBSnapshotIdentifier),
		Engine:               aws.String(v.Spec.Engine),
		DBSubnetGroupName:    aws.String(dbSubnetGroupName),
		VpcSecurityGroupIds:  securityGroups,
		PubliclyAccessible:   aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:              aws.Bool(v.Spec.MultiAZ),
	}
	if v.Spec.StorageType != "" {
		restoreSnapshotInput.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		restoreSnapshotInput.Iops = aws.Int64(v.Spec.Iops)
	}
	if v.Spec.DBParameterGroupName != "" {
		restoreSnapshotInput.DBParameterGroupName = aws.String(v.Spec.DBParameterGroupName)
	}

	modifyInstance := false
	modifyInstanceInput := &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier: aws.String(v.Name + "-" + v.Namespace),
	}

	if password != "" {
		modifyInstanceInput.MasterUserPassword = aws.String(password)
		modifyInstance = true
	}
	if v.Spec.Size > 0 {
		modifyInstanceInput.AllocatedStorage = aws.Int64(v.Spec.Size)
		modifyInstance = true
	}
	if v.Spec.BackupRetentionPeriod != nil {
		modifyInstanceInput.BackupRetentionPeriod = aws.Int64(*v.Spec.BackupRetentionPeriod)
		modifyInstance = true
	}

	if modifyInstance {
		return restoreSnapshotInput, modifyInstanceInput
	} else {
		return restoreSnapshotInput, nil
	}
}
