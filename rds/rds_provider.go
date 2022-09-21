package rds

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/crd"
	"github.com/sorenmat/k8s-rds/provider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type RDS struct {
	EC2             *ec2.Client
	Config          aws.Config
	Subnets         []string
	SecurityGroups  []string
	VpcId           string
	ServiceProvider provider.ServiceProvider
}

func New(ctx context.Context, publiclyAccessible bool, kc *kubernetes.Clientset) (provider.DatabaseProvider, error) {
	cfg, err := ec2config(ctx, kc)
	if err != nil {
		log.Fatal("unable to create a client for EC2 ", err)
	}

	ec2client := ec2.NewFromConfig(*cfg)

	nodeInfo, err := describeNodeEC2Instance(ctx, kc, ec2client)
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable AWS metadata")
	}
	vpcId := *nodeInfo.Reservations[0].Instances[0].VpcId

	log.Println("trying to get subnets")
	subnets, err := getSubnets(ctx, nodeInfo, ec2client, publiclyAccessible)
	if err != nil {
		return nil, fmt.Errorf("unable to get subnets from instance: %v", err)

	}

	log.Println("trying to get security groups")
	sgs, err := getSGS(ctx, kc, ec2client)
	if err != nil {
		return nil, fmt.Errorf("unable to get security groups from instance: %v", err)

	}

	r := RDS{
		EC2:            ec2client,
		Config:         *cfg,
		Subnets:        subnets,
		SecurityGroups: sgs,
		VpcId:          vpcId,
	}
	return &r, nil
}

func createDatabase(ctx context.Context, rdsCli *rds.Client, input *rds.CreateDBInstanceInput) error {
	_, err := rdsCli.CreateDBInstance(ctx, input)
	if err != nil {
		return errors.Wrap(err, "CreateDBInstance")
	}
	return nil
}

// waitForDBAvailability waits for db to become available
func waitForDBAvailability(ctx context.Context, dbInstanceIdentifier *string, rdsCli *rds.Client) error {
	if dbInstanceIdentifier == nil || *dbInstanceIdentifier == "" {
		return fmt.Errorf("error got empty db instance identifier")
	}
	log.Printf("Waiting for db instance %s to become available\n", *dbInstanceIdentifier)
	time.Sleep(5 * time.Second)
	ticker := time.NewTicker(time.Second * 10)
	timer := time.NewTimer(time.Minute * 30)
	defer ticker.Stop()
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("waited too much for db instance with db instance identifier %s to become available", *dbInstanceIdentifier)
		case <-ticker.C:
			k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: dbInstanceIdentifier}
			instance, err := rdsCli.DescribeDBInstances(ctx, k)
			if err != nil || len(instance.DBInstances) == 0 {
				return errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with db instance identifier %s", *dbInstanceIdentifier))
			}
			rdsdb := instance.DBInstances[0]
			if rdsdb.DBInstanceStatus != nil && *rdsdb.DBInstanceStatus == "available" {
				log.Printf("DB instance %s is now available\n", *dbInstanceIdentifier)
				return nil
			}
		}
	}
}

func isErrAs(err error, target interface{}) bool {
	if err == nil {
		return false
	}
	return errors.As(err, &target)
}

// CreateDatabase creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) CreateDatabase(ctx context.Context, db *crd.Database) (string, error) {
	// Ensure that the subnets for the DB is create or updated
	log.Println("Trying to find the correct subnets")
	subnetName, err := r.ensureSubnets(ctx)
	if err != nil {
		return "", err
	}
	pw := ""
	if db.Spec.DBClusterIdentifier != "" {
		log.Printf("getting secret: Name: %v Key: %v \n", db.Spec.Password.Name, db.Spec.Password.Key)
		pw, err = r.GetSecret(ctx, db.Namespace, db.Spec.Password.Name, db.Spec.Password.Key)
		if err != nil {
			return "", err
		}
	}
	input := convertSpecToInput(db, subnetName, r.SecurityGroups, pw)

	// search for the instance
	log.Printf("Trying to find db instance %v\n", *input.DBInstanceIdentifier)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}

	_, err = r.rdsclient().DescribeDBInstances(ctx, k)
	if isErrAs(err, &rdstypes.DBInstanceNotFoundFault{}) {
		if db.Spec.DBSnapshotIdentifier != "" {
			log.Printf("DB instance %v not found trying to restore it from snapshot with id: %s\n", *input.DBInstanceIdentifier, db.Spec.DBSnapshotIdentifier)
			// check for snapshot existence
			snapshotIdentifier := db.Spec.DBSnapshotIdentifier
			_, err = r.rdsclient().DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{DBSnapshotIdentifier: &snapshotIdentifier})
			if isErrAs(err, &rdstypes.DBSnapshotNotFoundFault{}) {
				// DB Snapshot was not found, creating the database
				log.Printf("DB Snapshot with identifier %s was not found trying to create new DB instance with name: %s\n", db.Spec.DBSnapshotIdentifier, *input.DBInstanceIdentifier)
				err = createDatabase(ctx, r.rdsclient(), input)
				if err != nil {
					return "", err
				}
			} else if err != nil {
				return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db snapshot with id %v", db.Spec.DBSnapshotIdentifier))
			} else {
				log.Printf("DB Snapshot with identifier %v was found trying to restore it\n", db.Spec.DBSnapshotIdentifier)
				restoreInput := convertSpecToRestoreFromSnapshotInput(db, subnetName, r.SecurityGroups)
				_, err = r.rdsclient().RestoreDBInstanceFromDBSnapshot(ctx, restoreInput)
				if err != nil {
					return "", errors.Wrap(err, "RestoreDBInstanceFromDBSnapshot")
				}
			}
		} else {
			log.Printf("DB instance %v not found trying to create it\n", *input.DBInstanceIdentifier)
			// seems like we didn't find a database with this name, let's create on
			err = createDatabase(ctx, r.rdsclient(), input)
			if err != nil {
				return "", err
			}
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", *input.DBInstanceIdentifier))
	}
	if err := waitForDBAvailability(ctx, input.DBInstanceIdentifier, r.rdsclient()); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("error while waiting for the DB %s availability", *input.DBInstanceIdentifier))
	}
	// Get the newly created database so we can get the endpoint
	dbHostname, err := getEndpoint(ctx, input.DBInstanceIdentifier, r.rdsclient())
	if err != nil {
		return "", err
	}
	return dbHostname, nil
}

func (r *RDS) UpdateDatabase(ctx context.Context, db *crd.Database) error {
	input := convertSpecToModifyInput(db)
	if err := waitForDBAvailability(ctx, input.DBInstanceIdentifier, r.rdsclient()); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while waiting for the DB %s availability", *input.DBInstanceIdentifier))
	}
	log.Printf("Trying to find db instance %v to update\n", *input.DBInstanceIdentifier)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}
	_, err := r.rdsclient().DescribeDBInstances(ctx, k)

	if err == nil {
		log.Printf("DB instance %v found trying to update it\n", *input.DBInstanceIdentifier)
		_, err := r.rdsclient().ModifyDBInstance(ctx, input)
		if err != nil {
			log.Printf("Updating database failed: ModifyDBInstance:%v\n", err)
			return errors.Wrap(err, "ModifyDBInstance")
		}
	} else {
		return errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", input.DBInstanceIdentifier))
	}

	if input.ApplyImmediately {
		log.Printf("Database modified and will be updated immediately")
	} else {
		log.Printf("Database modified and update is pending and will be executed during the next maintenance window")
	}
	if err := waitForDBAvailability(ctx, input.DBInstanceIdentifier, r.rdsclient()); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while waiting for the DB %s availability", *input.DBInstanceIdentifier))
	}
	return nil
}

func (r *RDS) getSubnetGroupName() string {
	return "db-subnetgroup-" + r.VpcId
}

// ensureSubnets is ensuring that we have created or updated the subnet according to the data from the CRD object
func (r *RDS) ensureSubnets(ctx context.Context) (string, error) {
	if len(r.Subnets) == 0 {
		log.Println("Error: unable to continue due to lack of subnets, perhaps we couldn't lookup the subnets")
	}
	subnetDescription := "RDS Subnet Group for VPC: " + r.VpcId
	subnetName := r.getSubnetGroupName()

	svc := r.rdsclient()

	sf := &rds.DescribeDBSubnetGroupsInput{DBSubnetGroupName: aws.String(subnetName)}
	_, err := svc.DescribeDBSubnetGroups(ctx, sf)
	log.Println("Subnets:", r.Subnets)
	if err != nil {
		// assume we didn't find it..
		subnet := &rds.CreateDBSubnetGroupInput{
			DBSubnetGroupDescription: aws.String(subnetDescription),
			DBSubnetGroupName:        aws.String(subnetName),
			SubnetIds:                r.Subnets,
			Tags:                     []rdstypes.Tag{{Key: aws.String("Warning"), Value: aws.String("Managed by k8s-rds.")}},
		}
		_, err := svc.CreateDBSubnetGroup(ctx, subnet)
		if err != nil {
			return "", errors.Wrap(err, "CreateDBSubnetGroup")
		}
	} else {
		log.Printf("Moving on seems like %v exsits", subnetName)
	}
	return subnetName, nil
}

func getEndpoint(ctx context.Context, dbName *string, svc *rds.Client) (string, error) {
	if dbName == nil || *dbName == "" {
		return "", fmt.Errorf("error got empty db instance name")
	}
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: dbName}

	instance, err := svc.DescribeDBInstances(ctx, k)
	if err != nil || len(instance.DBInstances) == 0 {
		return "", fmt.Errorf("wasn't able to describe the db instance with id %s", *dbName)
	}
	rdsdb := instance.DBInstances[0]
	if rdsdb.Endpoint == nil || rdsdb.Endpoint.Address == nil {
		return "", fmt.Errorf("couldn't get the endpoint for DB instance with id %s", *dbName)
	}
	dbHostname := *rdsdb.Endpoint.Address
	return dbHostname, nil
}

func dbSnapshotIdentifier(v *crd.Database, timestamp int64) string {
	return fmt.Sprintf("%s-%s-%d", v.Name, v.Namespace, timestamp)
}

func dbClusterSnapshotIdentifier(v *crd.DBCluster, timestamp int64) string {
	return fmt.Sprintf("%s-%s-%d", v.Name, v.Namespace, timestamp)
}

func convertSpecToDeleteInput(db *crd.Database, timestamp int64) *rds.DeleteDBInstanceInput {
	input := rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(dbidentifier(db)),
		SkipFinalSnapshot:    db.Spec.SkipFinalSnapshot,
	}
	if !db.Spec.SkipFinalSnapshot {
		input.FinalDBSnapshotIdentifier = aws.String(dbSnapshotIdentifier(db, timestamp))
	}
	return &input
}

func (r *RDS) DeleteDatabase(ctx context.Context, db *crd.Database) error {
	if db.Spec.DeleteProtection != nil && *db.Spec.DeleteProtection {
		log.Printf("Trying to delete a %v in %v which is a deleted protected database", db.Name, db.Namespace)
		return nil
	}
	// delete the database instance
	svc := r.rdsclient()

	input := convertSpecToDeleteInput(db, time.Now().UnixNano())
	if err := waitForDBAvailability(ctx, input.DBInstanceIdentifier, r.rdsclient()); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while waiting for the DB %s availability", *input.DBInstanceIdentifier))
	}
	_, err := svc.DeleteDBInstance(ctx, input)

	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("unable to delete database %v", *input.DBInstanceIdentifier))
		log.Println(err)
		return err
	}

	if !input.SkipFinalSnapshot && input.FinalDBSnapshotIdentifier != nil {
		log.Printf("Will create DB final snapshot: %v\n", *input.FinalDBSnapshotIdentifier)
	}

	log.Printf("Waiting for db instance %v to be deleted\n", *input.DBInstanceIdentifier)
	time.Sleep(5 * time.Second)

	// delete the subnet group attached to the instance
	subnetName := r.getSubnetGroupName()
	_, err = svc.DeleteDBSubnetGroup(ctx, &rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(subnetName)})
	if isErrAs(err, &rdstypes.InvalidDBSubnetGroupStateFault{}) {
		e := errors.Wrap(err, fmt.Sprintf("the DB subnet group %s cannot be deleted because it's in use", subnetName))
		log.Println(e)
		return e
	} else if isErrAs(err, &rdstypes.DBSubnetGroupNotFoundFault{}) {
		e := errors.Wrap(err, fmt.Sprintf("the DB subnet group %s doesn't refer to an existing DB subnet group", subnetName))
		log.Println(e)
		return e
	} else if isErrAs(err, &rdstypes.InvalidDBSubnetStateFault{}) {
		e := errors.Wrap(err, fmt.Sprintf("the DB subnet group %s isn't in the available state", subnetName))
		log.Println(e)
		return e
	} else if err != nil {
		e := errors.Wrap(err, fmt.Sprintf("unable to deelte the DB subnet group %s, unknown error", subnetName))
		log.Println(e)
		return e
	} else {
		log.Println("Deleted DBSubnet group: ", subnetName)
	}
	return nil
}

func convertSpecToClusterInput(v *crd.DBCluster, subnetName string, securityGroups []string, password string) *rds.CreateDBClusterInput {
	tags := toTags(v.Annotations, v.Labels)
	tags = append(tags, gettags(v.Spec.Tags)...)
	input := &rds.CreateDBClusterInput{
		DBClusterIdentifier: aws.String(v.Spec.DBClusterIdentifier),
		VpcSecurityGroupIds: securityGroups,
		Engine:              aws.String(v.Spec.Engine),
		MasterUserPassword:  aws.String(password),
		MasterUsername:      aws.String(v.Spec.MasterUsername),
		DBSubnetGroupName:   aws.String(subnetName),
		StorageEncrypted:    aws.Bool(v.Spec.StorageEncrypted),
		DeletionProtection:  aws.Bool(v.Spec.DeletionProtection),
		Tags:                tags,
	}

	if v.Spec.DBName != nil && *v.Spec.DBName != "" {
		input.DatabaseName = aws.String(*v.Spec.DBName)
	}

	if v.Spec.AllocatedStorage > 0 {
		input.AllocatedStorage = aws.Int32(int32(v.Spec.AllocatedStorage))
	}
	if v.Spec.PubliclyAccessible != nil {
		input.PubliclyAccessible = aws.Bool(*v.Spec.PubliclyAccessible)
	}
	if v.Spec.DBClusterInstanceClass != "" {
		input.DBClusterInstanceClass = aws.String(v.Spec.DBClusterInstanceClass)
	}
	if v.Spec.ServerlessV2ScalingConfiguration != nil {
		input.ServerlessV2ScalingConfiguration = &rdstypes.ServerlessV2ScalingConfiguration{
			MinCapacity: aws.Float64(*v.Spec.ServerlessV2ScalingConfiguration.MinCapacity),
			MaxCapacity: aws.Float64(*v.Spec.ServerlessV2ScalingConfiguration.MaxCapacity),
		}
	}
	if v.Spec.Port > 0 {
		input.Port = aws.Int32(int32(v.Spec.Port))
	}
	if v.Spec.BackupRetentionPeriod > 0 {
		input.BackupRetentionPeriod = aws.Int32(int32(v.Spec.BackupRetentionPeriod))
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int32(int32(v.Spec.Iops))
	}
	if v.Spec.EngineVersion != "" {
		input.EngineVersion = aws.String(v.Spec.EngineVersion)
	}
	return input
}

// waitForDBClusterAvailability
func waitForDBClusterAvailability(ctx context.Context, dbClusterIdentifier *string, rdsCli *rds.Client) error {
	if dbClusterIdentifier == nil || *dbClusterIdentifier == "" {
		return fmt.Errorf("error got empty db Cluster Identifier")
	}
	log.Printf("Waiting for db cluster instance with ID %s  to become available\n", *dbClusterIdentifier)
	time.Sleep(5 * time.Second)
	ticker := time.NewTicker(time.Second * 10)
	timer := time.NewTimer(time.Minute * 30)
	defer ticker.Stop()
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("waited too much for db cluster instance with ID %s to become available", *dbClusterIdentifier)
		case <-ticker.C:
			k := &rds.DescribeDBClustersInput{DBClusterIdentifier: dbClusterIdentifier}
			instance, err := rdsCli.DescribeDBClusters(ctx, k)
			if err != nil || len(instance.DBClusters) == 0 {
				return errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db cluster instance with ID %s", *dbClusterIdentifier))
			}
			rdsdb := instance.DBClusters[0]
			if rdsdb.Status != nil && *rdsdb.Status == "available" {
				log.Printf("DB cluster %s is now available\n", *dbClusterIdentifier)
				return nil
			}
		}
	}
}

func createDBCluster(ctx context.Context, rdsCli *rds.Client, input *rds.CreateDBClusterInput) error {
	_, err := rdsCli.CreateDBCluster(ctx, input)
	if err != nil {
		return errors.Wrap(err, "CreateDBCluster")
	}
	return nil
}

func convertSpecToRestoreClusterFromSnapshotInput(v *crd.DBCluster, subnetName string, securityGroups []string) *rds.RestoreDBClusterFromSnapshotInput {
	tags := toTags(v.Annotations, v.Labels)
	tags = append(tags, gettags(v.Spec.Tags)...)

	input := &rds.RestoreDBClusterFromSnapshotInput{
		SnapshotIdentifier:  aws.String(v.Spec.SnapshotIdentifier),
		DBClusterIdentifier: aws.String(v.Spec.DBClusterIdentifier),
		VpcSecurityGroupIds: securityGroups,
		Engine:              aws.String(v.Spec.Engine),
		DBSubnetGroupName:   aws.String(subnetName),
		DeletionProtection:  aws.Bool(v.Spec.DeletionProtection),
		Tags:                tags,
	}

	if v.Spec.DBName != nil && *v.Spec.DBName != "" {
		input.DatabaseName = aws.String(*v.Spec.DBName)
	}

	if v.Spec.PubliclyAccessible != nil {
		input.PubliclyAccessible = aws.Bool(*v.Spec.PubliclyAccessible)
	}
	if v.Spec.DBClusterInstanceClass != "" {
		input.DBClusterInstanceClass = aws.String(v.Spec.DBClusterInstanceClass)
	}
	if v.Spec.ServerlessV2ScalingConfiguration != nil {
		input.ServerlessV2ScalingConfiguration = &rdstypes.ServerlessV2ScalingConfiguration{
			MinCapacity: aws.Float64(*v.Spec.ServerlessV2ScalingConfiguration.MinCapacity),
			MaxCapacity: aws.Float64(*v.Spec.ServerlessV2ScalingConfiguration.MaxCapacity),
		}
	}
	if v.Spec.Port > 0 {
		input.Port = aws.Int32(int32(v.Spec.Port))
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int32(int32(v.Spec.Iops))
	}
	if v.Spec.EngineVersion != "" {
		input.EngineVersion = aws.String(v.Spec.EngineVersion)
	}
	return input
}

func (r *RDS) CreateDBCluster(ctx context.Context, cluster *crd.DBCluster) (string, error) {
	log.Println("Trying to find the correct subnets")
	subnetName, err := r.ensureSubnets(ctx)
	if err != nil {
		return "", err
	}

	log.Printf("getting secret: Name: %v Key: %v \n", cluster.Spec.MasterUserPassword.Name, cluster.Spec.MasterUserPassword.Key)
	pw, err := r.GetSecret(ctx, cluster.Namespace, cluster.Spec.MasterUserPassword.Name, cluster.Spec.MasterUserPassword.Key)
	if err != nil {
		return "", err
	}
	input := convertSpecToClusterInput(cluster, subnetName, r.SecurityGroups, pw)

	// search for the instance
	log.Printf("Trying to find db cluster %v\n", *input.DBClusterIdentifier)
	k := &rds.DescribeDBClustersInput{DBClusterIdentifier: input.DBClusterIdentifier}

	_, err = r.rdsclient().DescribeDBClusters(ctx, k)
	if isErrAs(err, &rdstypes.DBClusterNotFoundFault{}) {
		if cluster.Spec.SnapshotIdentifier != "" {
			log.Printf("DB cluster instance %v not found trying to restore it from snapshot with id: %s\n", cluster.Spec.DBClusterIdentifier, cluster.Spec.SnapshotIdentifier)
			// check for snapshot existence
			snapshotIdentifier := cluster.Spec.SnapshotIdentifier
			_, err = r.rdsclient().DescribeDBClusterSnapshots(ctx, &rds.DescribeDBClusterSnapshotsInput{DBClusterSnapshotIdentifier: &snapshotIdentifier})
			if isErrAs(err, &rdstypes.DBClusterSnapshotNotFoundFault{}) {
				// DB cluster  Snapshot was not found, creating the cluster
				log.Printf("DB cluster Snapshot with identifier %s was not found trying to create new DB instance with name: %s\n", cluster.Spec.SnapshotIdentifier, cluster.Spec.DBClusterIdentifier)
				err := createDBCluster(ctx, r.rdsclient(), input)
				if err != nil {
					return "", err
				}
			} else if err != nil {
				return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db snapshot with id %v", cluster.Spec.SnapshotIdentifier))
			} else {
				log.Printf("DB Snapshot with identifier %v was found trying to restore it\n", cluster.Spec.SnapshotIdentifier)
				restoreInput := convertSpecToRestoreClusterFromSnapshotInput(cluster, subnetName, r.SecurityGroups)
				_, err = r.rdsclient().RestoreDBClusterFromSnapshot(ctx, restoreInput)
				if err != nil {
					return "", errors.Wrap(err, "RestoreDBClusterFromSnapshot")
				}
			}
		} else {
			log.Printf("DB Cluster instance %v not found trying to create it\n", input.DBClusterIdentifier)
			err := createDBCluster(ctx, r.rdsclient(), input)
			if err != nil {
				return "", err
			}
		}

	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", input.DBClusterIdentifier))
	}
	if err := waitForDBClusterAvailability(ctx, input.DBClusterIdentifier, r.rdsclient()); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("error while waiting for the DB %s availability", *input.DatabaseName))
	}
	instance, err := r.rdsclient().DescribeDBClusters(ctx, k)
	if err != nil {
		return "", nil
	}
	rdsdb := instance.DBClusters[0]
	if rdsdb.Endpoint == nil {
		return "", nil
	}
	return *rdsdb.Endpoint, nil
}

func convertSpecToModifyClusterInput(v *crd.DBCluster, password string) *rds.ModifyDBClusterInput {
	input := &rds.ModifyDBClusterInput{
		DBClusterIdentifier: aws.String(v.Spec.DBClusterIdentifier),
		MasterUserPassword:  aws.String(password),
		DeletionProtection:  aws.Bool(v.Spec.DeletionProtection),
	}
	if v.Spec.AllocatedStorage > 0 {
		input.AllocatedStorage = aws.Int32(int32(v.Spec.AllocatedStorage))
	}
	if v.Spec.DBClusterInstanceClass != "" {
		input.DBClusterInstanceClass = aws.String(v.Spec.DBClusterInstanceClass)
	}
	if v.Spec.ServerlessV2ScalingConfiguration != nil {
		input.ServerlessV2ScalingConfiguration = &rdstypes.ServerlessV2ScalingConfiguration{
			MinCapacity: aws.Float64(*v.Spec.ServerlessV2ScalingConfiguration.MinCapacity),
			MaxCapacity: aws.Float64(*v.Spec.ServerlessV2ScalingConfiguration.MaxCapacity),
		}
	}
	if v.Spec.Port > 0 {
		input.Port = aws.Int32(int32(v.Spec.Port))
	}
	if v.Spec.BackupRetentionPeriod > 0 {
		input.BackupRetentionPeriod = aws.Int32(int32(v.Spec.BackupRetentionPeriod))
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int32(int32(v.Spec.Iops))
	}
	if v.Spec.EngineVersion != "" {
		input.EngineVersion = aws.String(v.Spec.EngineVersion)
	}
	return input
}

func (r *RDS) UpdateDBCluster(ctx context.Context, cluster *crd.DBCluster) error {
	pw, err := r.GetSecret(ctx, cluster.Namespace, cluster.Spec.MasterUserPassword.Name, cluster.Spec.MasterUserPassword.Key)
	if err != nil {
		return err
	}
	// to avoid InvalidDBClusterStateFault: Cannot modify engine version without a healthy primary instance in DB cluster
	if err := waitForDBClusterAvailability(ctx, &cluster.Spec.DBClusterIdentifier, r.rdsclient()); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while waiting for the DB cluster %s availability", cluster.Spec.DBClusterIdentifier))
	}
	input := convertSpecToModifyClusterInput(cluster, pw)

	log.Printf("Trying to find db cluster instance %v to update\n", cluster.Spec.DBClusterIdentifier)
	k := &rds.DescribeDBClustersInput{DBClusterIdentifier: input.DBClusterIdentifier}
	_, err = r.rdsclient().DescribeDBClusters(ctx, k)

	if err == nil {
		log.Printf("DB cluster instance %v found trying to update it\n", cluster.Spec.DBClusterIdentifier)
		_, err := r.rdsclient().ModifyDBCluster(ctx, input)
		if err != nil {
			log.Printf("Updating DB cluster failed: ModifyDBCluster:%v\n", err)
			return errors.Wrap(err, "ModifyDBCluster")
		}
	} else {
		return errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db cluster instance with id %v", input.DBClusterIdentifier))
	}

	if input.ApplyImmediately {
		log.Printf("DB cluster modified and will be updated immediately")
	} else {
		log.Printf("DB cluster modified and update is pending and will be executed during the next maintenance window")
	}
	if err := waitForDBClusterAvailability(ctx, input.DBClusterIdentifier, r.rdsclient()); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while waiting for the DB cluster %s availability", *input.DBClusterIdentifier))
	}
	return nil
}

func convertSpecToClusterDeleteInput(cluster *crd.DBCluster, timestamp int64) *rds.DeleteDBClusterInput {
	input := rds.DeleteDBClusterInput{
		DBClusterIdentifier: aws.String(cluster.Spec.DBClusterIdentifier),
		SkipFinalSnapshot:   cluster.Spec.SkipFinalSnapshot,
	}
	if !cluster.Spec.SkipFinalSnapshot {
		input.FinalDBSnapshotIdentifier = aws.String(dbClusterSnapshotIdentifier(cluster, timestamp))
	}
	return &input
}

func (r *RDS) DeleteDBCluster(ctx context.Context, cluster *crd.DBCluster) error {
	if cluster.Spec.DeletionProtection {
		log.Printf("Trying to delete a %v in %v which is a deleted protected cluster", cluster.Name, cluster.Namespace)
		return nil
	}
	svc := r.rdsclient()
	if err := waitForDBClusterAvailability(ctx, &cluster.Spec.DBClusterIdentifier, svc); err != nil {
		return errors.Wrap(err, fmt.Sprintf("error while waiting for the DB cluster %s availability", cluster.Spec.DBClusterIdentifier))
	}
	// before deleting the clutser, let's remove all db instances in it.
	k := &rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String(cluster.Spec.DBClusterIdentifier)}
	clusters, err := r.rdsclient().DescribeDBClusters(ctx, k)
	if err != nil || len(clusters.DBClusters) == 0 {
		err := errors.Wrap(err, fmt.Sprintf("unable to find the cluster %v", cluster.Spec.DBClusterIdentifier))
		log.Println(err)
		return err
	}
	dbs := clusters.DBClusters[0].DBClusterMembers
	for _, db := range dbs {
		k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: db.DBInstanceIdentifier}
		describeDBInstance, err := r.rdsclient().DescribeDBInstances(ctx, k)
		if err != nil || len(describeDBInstance.DBInstances) == 0 {
			err := errors.Wrap(err, fmt.Sprintf("unable to find the db instance %v in cluster %v", *db.DBInstanceIdentifier, cluster.Spec.DBClusterIdentifier))
			log.Println(err)
			continue
		}
		if describeDBInstance.DBInstances[0].DBInstanceStatus != nil && *describeDBInstance.DBInstances[0].DBInstanceStatus == "deleting" {
			continue
		}

		_ = waitForDBAvailability(ctx, db.DBInstanceIdentifier, r.rdsclient())
		input := &rds.DeleteDBInstanceInput{
			DBInstanceIdentifier: describeDBInstance.DBInstances[0].DBInstanceIdentifier,
		}
		_, err = svc.DeleteDBInstance(ctx, input)
		if err != nil {
			err := errors.Wrap(err, fmt.Sprintf("unable to delete database %v in cluster %v", *describeDBInstance.DBInstances[0].DBInstanceIdentifier, cluster.Spec.DBClusterIdentifier))
			log.Println(err)
			continue
		} else {
			log.Printf("deleted database %s in cluster %s\n", *describeDBInstance.DBInstances[0].DBInstanceIdentifier, cluster.Spec.DBClusterIdentifier)
		}
	}

	input := convertSpecToClusterDeleteInput(cluster, time.Now().UnixNano())
	_, err = svc.DeleteDBCluster(ctx, input)

	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("unable to delete cluster %v", cluster.Spec.DBClusterIdentifier))
		log.Println(err)
		return err
	}

	if !input.SkipFinalSnapshot && input.FinalDBSnapshotIdentifier != nil {
		log.Printf("Will create DB cluster final snapshot: %v\n", *input.FinalDBSnapshotIdentifier)
	}

	log.Printf("Waiting for db cluster %v to be deleted\n", cluster.Spec.DBClusterIdentifier)
	time.Sleep(5 * time.Second)

	// delete the subnet group attached to the instance
	subnetName := r.getSubnetGroupName()
	_, err = svc.DeleteDBSubnetGroup(ctx, &rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(subnetName)})
	if isErrAs(err, &rdstypes.InvalidDBSubnetGroupStateFault{}) {
		e := errors.Wrap(err, fmt.Sprintf("the DB subnet group %s cannot be deleted because it's in use", subnetName))
		log.Println(e)
		return e
	} else if isErrAs(err, &rdstypes.DBSubnetGroupNotFoundFault{}) {
		e := errors.Wrap(err, fmt.Sprintf("the DB subnet group %s doesn't refer to an existing DB subnet group", subnetName))
		log.Println(e)
		return e
	} else if isErrAs(err, &rdstypes.InvalidDBSubnetStateFault{}) {
		e := errors.Wrap(err, fmt.Sprintf("the DB subnet group %s isn't in the available state", subnetName))
		log.Println(e)
		return e
	} else if err != nil {
		e := errors.Wrap(err, fmt.Sprintf("unable to deelte the DB subnet group %s, unknown error", subnetName))
		log.Println(e)
		return e
	} else {
		log.Println("Deleted DBSubnet group: ", subnetName)
	}
	return nil
}

func (r *RDS) rdsclient() *rds.Client {
	return rds.NewFromConfig(r.Config)
}
func dbidentifier(v *crd.Database) string {
	if v.Spec.DBInstanceIdentifier != "" {
		return v.Spec.DBInstanceIdentifier
	}
	return v.Name + "-" + v.Namespace
}

const (
	maxTagLengthAllowed = 255
	tagRegexp           = `^kube.*$`
)

func toTags(annotations, labels map[string]string) []rdstypes.Tag {
	var tags []rdstypes.Tag
	r := regexp.MustCompile(tagRegexp)

	for k, v := range annotations {
		if len(k) > maxTagLengthAllowed || len(v) > maxTagLengthAllowed ||
			r.Match([]byte(k)) {
			log.Printf("WARNING: Not Adding annotation KV to tags: %v %v", k, v)
			continue
		}

		tags = append(tags, rdstypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	for k, v := range labels {
		if len(k) > maxTagLengthAllowed || len(v) > maxTagLengthAllowed {
			log.Printf("WARNING: Not Adding CRD labels KV to tags: %v %v", k, v)
			continue
		}

		tags = append(tags, rdstypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	return tags
}

func gettags(tagsStr string) []rdstypes.Tag {
	var tags []rdstypes.Tag
	if tagsStr == "" {
		return tags
	}
	for _, v := range strings.Split(tagsStr, ",") {
		kv := strings.Split(v, "=")

		tags = append(tags, rdstypes.Tag{Key: aws.String(strings.TrimSpace(kv[0])), Value: aws.String(strings.TrimSpace(kv[1]))})
	}
	return tags
}

func convertSpecToRestoreFromSnapshotInput(v *crd.Database, subnetName string, securityGroups []string) *rds.RestoreDBInstanceFromDBSnapshotInput {
	tags := toTags(v.Annotations, v.Labels)
	tags = append(tags, gettags(v.Spec.Tags)...)

	input := &rds.RestoreDBInstanceFromDBSnapshotInput{
		DBSnapshotIdentifier: aws.String(v.Spec.DBSnapshotIdentifier),
		DBInstanceClass:      aws.String(v.Spec.Class),
		DBInstanceIdentifier: aws.String(dbidentifier(v)),
		VpcSecurityGroupIds:  securityGroups,
		Engine:               aws.String(v.Spec.Engine),
		DBSubnetGroupName:    aws.String(subnetName),
		PubliclyAccessible:   aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:              aws.Bool(v.Spec.MultiAZ),
		Tags:                 tags,
	}
	if v.Spec.DeleteProtection != nil {
		input.DeletionProtection = aws.Bool(*v.Spec.DeleteProtection)
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int32(int32(v.Spec.Iops))
	}
	return input
}

func convertSpecToInput(v *crd.Database, subnetName string, securityGroups []string, password string) *rds.CreateDBInstanceInput {
	tags := toTags(v.Annotations, v.Labels)
	tags = append(tags, gettags(v.Spec.Tags)...)

	input := &rds.CreateDBInstanceInput{
		DBInstanceClass:      aws.String(v.Spec.Class),
		DBInstanceIdentifier: aws.String(dbidentifier(v)),
		Engine:               aws.String(v.Spec.Engine),
		DBSubnetGroupName:    aws.String(subnetName),
		PubliclyAccessible:   aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:              aws.Bool(v.Spec.MultiAZ),
		Tags:                 tags,
	}

	if v.Spec.DBClusterIdentifier != "" {
		input.DBClusterIdentifier = aws.String(v.Spec.DBClusterIdentifier)
	} else {

		// avoid  InvalidParameterCombination: The requested DB Instance will be a member of a DB Cluster. Set master user password for the DB Cluster
		input.MasterUsername = aws.String(v.Spec.Username)
		input.MasterUserPassword = aws.String(password)
		input.VpcSecurityGroupIds = securityGroups
		input.StorageEncrypted = aws.Bool(v.Spec.StorageEncrypted)
	}
	if v.Spec.DBName != nil && *v.Spec.DBName != "" {
		input.DBName = aws.String(*v.Spec.DBName)
	}
	if v.Spec.DeleteProtection != nil {
		input.DeletionProtection = aws.Bool(*v.Spec.DeleteProtection)
	}
	if v.Spec.BackupRetentionPeriod != nil && *v.Spec.BackupRetentionPeriod > 0 {
		input.BackupRetentionPeriod = aws.Int32(int32(*v.Spec.BackupRetentionPeriod))
	}
	if v.Spec.Size != nil && *v.Spec.Size > 0 {
		input.AllocatedStorage = aws.Int32(int32(*v.Spec.Size))
	}
	if v.Spec.MaxAllocatedSize != nil && *v.Spec.MaxAllocatedSize > 0 {
		input.MaxAllocatedStorage = aws.Int32(int32(*v.Spec.MaxAllocatedSize))
	}
	if v.Spec.Version != "" {
		input.EngineVersion = aws.String(v.Spec.Version)
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int32(int32(v.Spec.Iops))
	}
	return input
}

func convertSpecToModifyInput(v *crd.Database) *rds.ModifyDBInstanceInput {
	input := &rds.ModifyDBInstanceInput{
		DBInstanceClass:      aws.String(v.Spec.Class),
		ApplyImmediately:     v.Spec.ApplyImmediately,
		DBInstanceIdentifier: aws.String(dbidentifier(v)),
		PubliclyAccessible:   aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:              aws.Bool(v.Spec.MultiAZ),
	}
	if v.Spec.DeleteProtection != nil {
		input.DeletionProtection = aws.Bool(*v.Spec.DeleteProtection)
	}
	if v.Spec.Size != nil && *v.Spec.Size > 0 {
		input.AllocatedStorage = aws.Int32(int32(*v.Spec.Size))
	}
	if v.Spec.MaxAllocatedSize != nil && *v.Spec.MaxAllocatedSize > 0 {
		input.MaxAllocatedStorage = aws.Int32(int32(*v.Spec.MaxAllocatedSize))
	}
	if v.Spec.BackupRetentionPeriod != nil && *v.Spec.BackupRetentionPeriod > 0 {
		input.BackupRetentionPeriod = aws.Int32(int32(*v.Spec.BackupRetentionPeriod))
	}
	if v.Spec.Version != "" {
		input.EngineVersion = aws.String(v.Spec.Version)
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int32(int32(v.Spec.Iops))
	}
	return input
}

//DescribeInstancesResponse
// describeNodeEC2Instance returns the AWS Metadata for the firt Node from the cluster
func describeNodeEC2Instance(ctx context.Context, kubectl *kubernetes.Clientset, svc *ec2.Client) (*ec2.DescribeInstancesOutput, error) {
	nodes, err := kubectl.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""

	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}

	// take the first one, we assume that all nodes are created in the same VPC
	name = getIDFromProvider(nodes.Items[0].Spec.ProviderID)

	log.Printf("Taking subnets from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	nodeInfo, err := svc.DescribeInstances(ctx, params)

	if err != nil {
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	if len(nodeInfo.Reservations) == 0 {
		log.Println(err)
		return nil, fmt.Errorf("unable to describe AWS instance")
	}

	return nodeInfo, nil
}

// getSubnets returns a list of subnets within the VPC from the Kubernetes Node.
func getSubnets(ctx context.Context, nodeInfo *ec2.DescribeInstancesOutput, svc *ec2.Client, public bool) ([]string, error) {
	var result []string
	vpcID := nodeInfo.Reservations[0].Instances[0].VpcId
	for _, v := range nodeInfo.Reservations[0].Instances[0].SecurityGroups {
		log.Println("Security groupid: ", *v.GroupId)
	}
	log.Printf("Found VPC %v will search for subnet in that VPC\n", *vpcID)

	//DescribeSubnetsRequest
	subnets, err := svc.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{Filters: []ec2types.Filter{{Name: aws.String("vpc-id"), Values: []string{*vpcID}}}})
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to describe subnet in VPC %v", *vpcID))
	}
	for _, sn := range subnets.Subnets {
		if sn.MapPublicIpOnLaunch != nil && *sn.MapPublicIpOnLaunch == public {
			result = append(result, *sn.SubnetId)
		} else {
			log.Printf("Skipping subnet %v since it's public state was %v and we were looking for %v\n", *sn.SubnetId, sn.MapPublicIpOnLaunch, public)
		}
	}

	log.Printf("Found the follwing subnets: ")
	for _, v := range result {
		log.Printf(v + " ")
	}
	return result, nil
}

func getIDFromProvider(x string) string {
	pos := strings.LastIndex(x, "/") + 1
	name := x[pos:]
	return name
}
func getSGS(ctx context.Context, kubectl *kubernetes.Clientset, svc *ec2.Client) ([]string, error) {

	nodes, err := kubectl.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = getIDFromProvider(nodes.Items[0].Spec.ProviderID)
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Printf("Taking security groups from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	res, err := svc.DescribeInstances(ctx, params)
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	log.Println("got instance response")

	var result []string
	if len(res.Reservations) >= 1 {
		for _, v := range res.Reservations[0].Instances[0].SecurityGroups {
			log.Println("Security groupid: ", *v.GroupId)
			result = append(result, *v.GroupId)
		}
	}

	log.Printf("Found the follwing security groups: ")
	for _, v := range result {
		log.Printf(v + " ")
	}
	return result, nil
}

func ec2config(ctx context.Context, kubectl *kubernetes.Clientset) (*aws.Config, error) {
	nodes, err := kubectl.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""
	region := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = getIDFromProvider(nodes.Items[0].Spec.ProviderID)
		region = nodes.Items[0].Labels["failure-domain.beta.kubernetes.io/region"]
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Printf("Found node with ID: %v in region %v", name, region)

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	// Set the AWS Region that the service clients should use
	cfg.Region = region
	return &cfg, nil
}
