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
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 10,
			Class:              "db.t2.micro",
			DBName:             "database_name",
			Engine:             "postgres",
			Iops:               1000,
			MultiAZ:            true,
			Password:           v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible: false,
			Size:               20,
			StorageEncrypted:   true,
			StorageType:        "gp2",
			Username:           "dbuser",
		},
	}
	j, err := yaml.Marshal(d)
	assert.NoError(t, err)
	assert.NotNil(t, j)
	fmt.Println(string(j))

}
