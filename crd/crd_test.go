package crd

import (
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMarshal(t *testing.T) {
	backupRetentionPeriod := int64(10)
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: &backupRetentionPeriod,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			DBSnapshotIdentifier:  "rds-snapshot",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			SecurityGroups:        []string{"DBSnapshotIdentifier"},
			Subnets: []string{
				"subnet-0a378a72330fea864",
				"subnet-0c4fb739e201fc2a3",
				"subnet-0739bd9fa24055c4b",
			},
			DBSubnetGroupName:    "rds-subnet-group",
			DBParameterGroupName: "default.postgres10",
			Size:                 20,
			StorageEncrypted:     true,
			StorageType:          "gp2",
			Username:             "dbuser",
		},
	}
	j, err := yaml.Marshal(d)
	assert.NoError(t, err)
	assert.NotNil(t, j)
	fmt.Println(string(j))

}
