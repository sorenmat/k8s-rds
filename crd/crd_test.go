package crd

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/xeipuuv/gojsonschema"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMarshal(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"}, Spec: DatabaseSpec{BackupRetentionPeriod: 10,
			Class:              "db.t2.micro",
			DBName:             "database_name",
			Engine:             "postgres",
			MultiAZ:            true,
			Password:           v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible: false,
			Size:               20,
			MaxAllocatedSize:   20,
			StorageEncrypted:   true,
			StorageType:        "gp2",
			Username:           "dbuser",
			Provider:           "local",
		},
	}
	j, err := yaml.Marshal(d)
	assert.NoError(t, err)
	assert.NotNil(t, j)
	fmt.Println(string(j))

}

func TestCRDValidationWithValidInput(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 10,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  50,
			MaxAllocatedSize:      50,
			StorageEncrypted:      true,
			StorageType:           "gp2",
			Username:              "dbuser",
			Provider:              "local",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.True(t, result.Valid(), result.Errors())
}

func TestCaseInsensitiveInput(t *testing.T) {
	// in the test.yaml, you can see maxallocatedsize instead of MaxAllocatedSize
	yamlFile, err := ioutil.ReadFile("test.yaml")
	assert.NoError(t, err)
	db := Database{}
	err = yaml.Unmarshal(yamlFile, &db)
	assert.NoError(t, err)
	assert.Equal(t, int(db.Spec.MaxAllocatedSize), 200, "they should be equal")
	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(db)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.True(t, result.Valid(), result.Errors())
}

func TestDatabaseSizeIsTooSmall(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 10,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  10,
			MaxAllocatedSize:      10,
			StorageEncrypted:      true,
			StorageType:           "gp2",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestDatabaseSizeIsTooBig(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 10,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65000,
			MaxAllocatedSize:      65000,
			StorageEncrypted:      true,
			StorageType:           "gp2",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestBackupRetentionPeriodIsTooLong(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 36,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65,
			MaxAllocatedSize:      65,
			StorageEncrypted:      true,
			StorageType:           "gp2",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestInvalidDatabaseNameWithDashSeparator(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 30,
			Class:                 "db.t2.micro",
			DBName:                "database-name",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65,
			MaxAllocatedSize:      65,
			StorageEncrypted:      true,
			StorageType:           "gp2",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestInvalidDatabaseNameStartingWithANumber(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 30,
			Class:                 "db.t2.micro",
			DBName:                "1database_name",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65,
			MaxAllocatedSize:      65,
			StorageEncrypted:      true,
			StorageType:           "gp2",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestInvalidUsername(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 30,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65,
			MaxAllocatedSize:      65,
			StorageEncrypted:      true,
			StorageType:           "gp2",
			Username:              "db-user",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestInvalidStorageType(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 30,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			Iops:                  1000,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65,
			MaxAllocatedSize:      65,
			StorageEncrypted:      true,
			StorageType:           "io2",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestIopsTooSmall(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 30,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			Iops:                  999,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65,
			MaxAllocatedSize:      65,
			StorageEncrypted:      true,
			StorageType:           "io1",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}

func TestIopsTooBig(t *testing.T) {
	d := Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "my_db", Namespace: "default"},
		TypeMeta:   meta_v1.TypeMeta{Kind: "Database", APIVersion: "k8s.io/v1"},
		Spec: DatabaseSpec{
			BackupRetentionPeriod: 30,
			Class:                 "db.t2.micro",
			DBName:                "database_name",
			Engine:                "postgres",
			Iops:                  80001,
			MultiAZ:               true,
			Password:              v1.SecretKeySelector{Key: "key", LocalObjectReference: v1.LocalObjectReference{Name: "DB-Secret"}},
			PubliclyAccessible:    false,
			Size:                  65,
			MaxAllocatedSize:      65,
			StorageEncrypted:      true,
			StorageType:           "io1",
			Username:              "dbuser",
		},
	}

	loader := gojsonschema.NewGoLoader(NewDatabaseCRD().Spec.Versions[0].Schema.OpenAPIV3Schema)
	documentLoader := gojsonschema.NewGoLoader(d)

	result, err := gojsonschema.Validate(loader, documentLoader)
	assert.NoError(t, err)
	assert.False(t, result.Valid(), result.Errors())
}
