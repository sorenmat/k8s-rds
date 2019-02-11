package rds

import (
	"testing"

	"github.com/cloud104/k8s-rds/crd"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
)

func TestConvertSpecToInput(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			DBName:             "mydb",
			Engine:             "postgres",
			Username:           "myuser",
			Class:              "db.t2.micro",
			Size:               100,
			MultiAZ:            true,
			PubliclyAccessible: true,
			StorageEncrypted:   true,
			StorageType:        "bad",
			Iops:               1000,
			Password:           v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	i := convertSpecToInput(db, "mysubnet", []string{"sg-1234", "sg-4321"}, "mypassword")
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
}
