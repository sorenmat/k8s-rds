package rds

import (
	"testing"

	"github.com/sorenmat/k8s-rds/crd"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestConvertSpecToInput(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			DBName:                "mydb",
			Engine:                "postgres",
			Username:              "myuser",
			Class:                 "db.t2.micro",
			Size:                  100,
			MultiAZ:               true,
			PubliclyAccessible:    true,
			StorageEncrypted:      true,
			StorageType:           "bad",
			Iops:                  1000,
			DBParameterGroupName:  "default.postgres10",
			BackupRetentionPeriod: int64(10),
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
	//assert.Equal(t, "default.postgres10", *i.DBParameterGroupName)
	//assert.Equal(t, int64(10), *i.BackupRetentionPeriod)
}

func TestConvertSpecToRestoreSnapshotInput(t *testing.T) {
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
			BackupRetentionPeriod: int64(10),
			Password:              v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	restoreSnapshotInput, modifyInstanceInput := convertSpecToRestoreSnapshotInput(db, "mysubnet", []string{"sg-1234", "sg-4321"}, "mypassword")
	assert.Equal(t, "", *restoreSnapshotInput.DBName)
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

func TestConvertSpecToRestoreSnapshotInputDBName(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			DBName:                "mydb",
			DBSnapshotIdentifier:  "snapshot",
			DBParameterGroupName:  "default.oracle-ee-19",
			Engine:                "oracle-ee",
			Username:              "myuser",
			Class:                 "db.t2.micro",
			Size:                  100,
			MultiAZ:               true,
			PubliclyAccessible:    true,
			StorageEncrypted:      true,
			StorageType:           "bad",
			Iops:                  1000,
			BackupRetentionPeriod: int64(10),
			Password:              v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	restoreSnapshotInput, modifyInstanceInput := convertSpecToRestoreSnapshotInput(db, "mysubnet", []string{"sg-1234", "sg-4321"}, "mypassword")
	assert.Equal(t, "mydb", *restoreSnapshotInput.DBName)
	assert.Equal(t, "snapshot", *restoreSnapshotInput.DBSnapshotIdentifier)
	assert.Equal(t, "oracle-ee", *restoreSnapshotInput.Engine)
	assert.Equal(t, "db.t2.micro", *restoreSnapshotInput.DBInstanceClass)
	assert.Equal(t, true, *restoreSnapshotInput.PubliclyAccessible)
	assert.Equal(t, true, *restoreSnapshotInput.MultiAZ)
	assert.Equal(t, 2, len(restoreSnapshotInput.VpcSecurityGroupIds))
	assert.Equal(t, "bad", *restoreSnapshotInput.StorageType)
	assert.Equal(t, int64(1000), *restoreSnapshotInput.Iops)
	assert.Equal(t, "default.oracle-ee-19", *restoreSnapshotInput.DBParameterGroupName)
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
			BackupRetentionPeriod: int64(0),
			Password:              v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: ""}, Key: ""},
		},
	}
	restoreSnapshotInput, modifyInstanceInput := convertSpecToRestoreSnapshotInput(db, "mysubnet", []string{"sg-1234", "sg-4321"}, "")
	assert.Equal(t, "", *restoreSnapshotInput.DBName)
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

func TestGetIDFromProvider(t *testing.T) {
	x := getIDFromProvider("aws:///eu-west-1a/i-02ab67f4da79c3caa")
	assert.Equal(t, "i-02ab67f4da79c3caa", x)
}

// 270 characters length
const testRandString = "banjmdvgeezuadqvehvqaxxmzwykirejkwvktkxmvjdevcfhqootqyfdfvqatjiebglktdswnvzxcpnstvrurpfjfuxhsjvgogrnhazjizakttdncmjnbofvwcsccigfcyxzlunfndcjteuqmjpslqvefvobfnejjxtwbyrkcvsvqokkrskrryzbhhayegyuwhugyorkltmsipvznxkonqzzwihjdejqgzfjivjdqmieidkowryfjnnyrxszsyhnpfeepxyoliskexxpjtxn"

func TestToTags(t *testing.T) {
	tests := []struct {
		Annotations map[string]string
		Labels      map[string]string
		Result      map[string]string
	}{
		{
			Annotations: map[string]string{
				"annotation-key1-test": "test-value",
				"annotation-key2-test": "test-value",
			},
			Labels: map[string]string{
				"label-key1-test": "test-value",
				"label-key2-test": "test-value",
			},
			Result: map[string]string{
				"annotation-key1-test": "test-value",
				"annotation-key2-test": "test-value",
				"label-key1-test":      "test-value",
				"label-key2-test":      "test-value",
			},
		},

		{
			Annotations: map[string]string{
				"annotation-key1-test": "test-value",
				"kubectl-key2-test":    "test-value",
			},
			Labels: map[string]string{
				"kubectl-key1-test": "test-value",
				"label-key2-test":   "test-value",
			},
			Result: map[string]string{
				"annotation-key1-test": "test-value",
				"label-key2-test":      "test-value",
				"kubectl-key1-test":    "test-value",
			},
		},

		{
			Annotations: map[string]string{
				"annotation-key1-test": testRandString,
				"kubectl-key2-test":    "test-value",
			},
			Labels: map[string]string{
				"kubectl-key1-test": "test-value",
				"label-key2-test":   testRandString,
				"label-key3-test":   "test",
			},
			Result: map[string]string{
				"label-key3-test":   "test",
				"kubectl-key1-test": "test-value",
			},
		},
	}

	for testInd, test := range tests {

		tags := toTags(test.Annotations, test.Labels)

		if len(tags) != len(test.Result) {
			t.Fatalf("Not desired result %v != %v len(%v, %v): %v", tags, test.Result,
				len(tags), len(test.Result), testInd)
		}

		for _, tag := range tags {
			_, ok := test.Result[*tag.Key]
			if !ok {
				t.Fatalf("Not desired result in per component comparison %v != %v key %v: %v",
					tags, test.Result, *tag.Key, testInd)
			}

		}

	}
}

func TestTags(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			Tags: "key=value,key1=value1",
		},
	}
	tags := gettags(db)
	assert.NotNil(t, tags)
	assert.Equal(t, 2, len(tags))
	assert.Equal(t, "key", *tags[0].Key)
	assert.Equal(t, "value", *tags[0].Value)
	assert.Equal(t, "key1", *tags[1].Key)
	assert.Equal(t, "value1", *tags[1].Value)

}
func TestTagsWithSpaces(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			Tags: "key= value,   key1=value1",
		},
	}
	tags := gettags(db)
	assert.NotNil(t, tags)
	assert.Equal(t, 2, len(tags))
	assert.Equal(t, "key", *tags[0].Key)
	assert.Equal(t, "value", *tags[0].Value)
	assert.Equal(t, "key1", *tags[1].Key)
	assert.Equal(t, "value1", *tags[1].Value)

}
