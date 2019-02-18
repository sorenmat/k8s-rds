package rds

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/cloud104/k8s-rds/crd"
	"log"
)

type AWS struct {
	RDS *rds.RDS
}

func (a *AWS) RestoreDatabase(db *crd.Database) (string, error) {
	svc := a.RDS
	input := convertSpecToInputRestore(db)
	res := svc.RestoreDBInstanceFromDBSnapshotRequest(input)
	_, err := res.Send()

	if err != nil {
		log.Println(err)
		return "", nil
	}
	return "", nil
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

func convertSpecToInputCreate(v *crd.Database, subnetName string, securityGroups []string, password string) *rds.CreateDBInstanceInput {
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
