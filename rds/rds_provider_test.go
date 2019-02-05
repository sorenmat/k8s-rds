package rds

import (
	"testing"

	"github.com/sorenmat/k8s-rds/crd"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
)

func TestConvertSpecToInput(t *testing.T) {
	backupRetentionPeriod := int64(10)
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			DBName:                "mydb",
			DBParameterGroupName:  "default.postgres10",
			Engine:                "postgres",
			Username:              "myuser",
			Class:                 "db.t2.micro",
			Size:                  100,
			MultiAZ:               true,
			PubliclyAccessible:    true,
			StorageEncrypted:      true,
			StorageType:           "bad",
			Iops:                  1000,
			BackupRetentionPeriod: &backupRetentionPeriod,
			Password:              v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	i := convertSpecToInstanceInput(db, "mysubnet", []string{"sg-1234", "sg-4321"}, "mypassword")
	assert.Equal(t, "mydb", *i.DBName)
	assert.Equal(t, "postgres", *i.Engine)
	assert.Equal(t, "mypassword", *i.MasterUserPassword)
	assert.Equal(t, "myuser", *i.MasterUsername)
	assert.Equal(t, "db.t2.micro", *i.DBInstanceClass)
	assert.Equal(t, int64(100), *i.AllocatedStorage)
	assert.Equal(t, true, *i.PubliclyAccessible)
	assert.Equal(t, true, *i.MultiAZ)
	assert.Equal(t, true, *i.StorageEncrypted)
	assert.Equal(t, 2, len(i.VpcSecurityGroupIds))
	assert.Equal(t, "bad", *i.StorageType)
	assert.Equal(t, int64(1000), *i.Iops)
	assert.Equal(t, "default.postgres10", *i.DBParameterGroupName)
	assert.Equal(t, int64(10), *i.BackupRetentionPeriod)
}

func TestConvertSpecToRestoreSnapshotInput(t *testing.T) {
	backupRetentionPeriod := int64(10)
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			DBName:                "mydb",
			DBSnapshotIdentifier:  "snapshot",
			DBParameterGroupName:  "default.postgres10",
			Engine:                "postgres",
			Username:              "myuser",
			Class:                 "db.t2.micro",
			Size:                  100,
			MultiAZ:               true,
			PubliclyAccessible:    true,
			StorageEncrypted:      true,
			StorageType:           "bad",
			Iops:                  1000,
			BackupRetentionPeriod: &backupRetentionPeriod,
			Password:              v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	restoreSnapshotInput, modifyInstanceInput := convertSpecToRestoreSnapshotInput(db, "mysubnet", []string{"sg-1234", "sg-4321"}, "mypassword")
	assert.Equal(t, "mydb", *restoreSnapshotInput.DBName)
	assert.Equal(t, "snapshot", *restoreSnapshotInput.DBSnapshotIdentifier)
	assert.Equal(t, "postgres", *restoreSnapshotInput.Engine)
	assert.Equal(t, "db.t2.micro", *restoreSnapshotInput.DBInstanceClass)
	assert.Equal(t, true, *restoreSnapshotInput.PubliclyAccessible)
	assert.Equal(t, true, *restoreSnapshotInput.MultiAZ)
	assert.Equal(t, 2, len(restoreSnapshotInput.VpcSecurityGroupIds))
	assert.Equal(t, "bad", *restoreSnapshotInput.StorageType)
	assert.Equal(t, int64(1000), *restoreSnapshotInput.Iops)
	assert.Equal(t, "default.postgres10", *restoreSnapshotInput.DBParameterGroupName)

	assert.Equal(t, "mypassword", *modifyInstanceInput.MasterUserPassword)
	assert.Equal(t, int64(100), *modifyInstanceInput.AllocatedStorage)
	assert.Equal(t, int64(10), *modifyInstanceInput.BackupRetentionPeriod)
}

func TestConvertSpecToRestoreSnapshotInputNoModify(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			DBName:                "mydb",
			DBSnapshotIdentifier:  "snapshot",
			DBParameterGroupName:  "default.postgres10",
			Engine:                "postgres",
			Username:              "myuser",
			Class:                 "db.t2.micro",
			Size:                  0,
			MultiAZ:               true,
			PubliclyAccessible:    true,
			StorageEncrypted:      true,
			StorageType:           "bad",
			Iops:                  1000,
			BackupRetentionPeriod: nil,
			Password:              v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: ""}, Key: ""},
		},
	}
	restoreSnapshotInput, modifyInstanceInput := convertSpecToRestoreSnapshotInput(db, "mysubnet", []string{"sg-1234", "sg-4321"}, "")
	assert.Equal(t, "mydb", *restoreSnapshotInput.DBName)
	assert.Equal(t, "snapshot", *restoreSnapshotInput.DBSnapshotIdentifier)
	assert.Equal(t, "postgres", *restoreSnapshotInput.Engine)
	assert.Equal(t, "db.t2.micro", *restoreSnapshotInput.DBInstanceClass)
	assert.Equal(t, true, *restoreSnapshotInput.PubliclyAccessible)
	assert.Equal(t, true, *restoreSnapshotInput.MultiAZ)
	assert.Equal(t, 2, len(restoreSnapshotInput.VpcSecurityGroupIds))
	assert.Equal(t, "bad", *restoreSnapshotInput.StorageType)
	assert.Equal(t, int64(1000), *restoreSnapshotInput.Iops)
	assert.Equal(t, "default.postgres10", *restoreSnapshotInput.DBParameterGroupName)

	assert.Nil(t, modifyInstanceInput)
}
