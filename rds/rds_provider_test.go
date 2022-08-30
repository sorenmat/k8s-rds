package rds

import (
	"testing"

	"github.com/sorenmat/k8s-rds/crd"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertSpecToInput(t *testing.T) {
	db := &crd.Database{
		Spec: crd.DatabaseSpec{
			DBName:             "mydb",
			Engine:             "postgres",
			Username:           "myuser",
			Class:              "db.t2.micro",
			Size:               100,
			MaxAllocatedSize:   200,
			MultiAZ:            true,
			PubliclyAccessible: true,
			StorageEncrypted:   true,
			StorageType:        "bad",
			Version:            "9.6",
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
	assert.Equal(t, int32(100), *i.AllocatedStorage)
	assert.Equal(t, int32(200), *i.MaxAllocatedStorage)
	assert.Equal(t, true, *i.PubliclyAccessible)
	assert.Equal(t, true, *i.MultiAZ)
	assert.Equal(t, true, *i.StorageEncrypted)
	assert.Equal(t, 2, len(i.VpcSecurityGroupIds))
	assert.Equal(t, "bad", *i.StorageType)
	assert.Equal(t, int32(1000), *i.Iops)
	assert.Equal(t, "9.6", *i.EngineVersion)
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

func TestConvertSpecToDeleteInput_enabled(t *testing.T) {
	timestamp := int64(10202020202)
	input := convertSpecToDeleteInput(&crd.Database{
		Spec: crd.DatabaseSpec{
			SkipFinalSnapshot: true,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "mydb",
			Namespace: "myns",
		},
	}, timestamp)
	assert.Nil(t, input.FinalDBSnapshotIdentifier)
	assert.Equal(t, true, input.SkipFinalSnapshot)
}

func TestConvertSpecToDeleteInput_disabled(t *testing.T) {
	timestamp := int64(10202020202)
	input := convertSpecToDeleteInput(&crd.Database{
		Spec: crd.DatabaseSpec{
			SkipFinalSnapshot: false,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "mydb",
			Namespace: "myns",
		},
	}, timestamp)
	assert.NotNil(t, input.FinalDBSnapshotIdentifier)
	assert.Equal(t, "mydb-myns-10202020202", *input.FinalDBSnapshotIdentifier)
	assert.Equal(t, false, input.SkipFinalSnapshot)
}

func TestConvertSpecToModifyInput_withApplyImmediately(t *testing.T) {
	input := convertSpecToModifyInput(&crd.Database{
		Spec: crd.DatabaseSpec{
			MaxAllocatedSize: 50,
			Size:             21,
			ApplyImmediately: true,
			Class:            "db.t3.small",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "mydb",
			Namespace: "myns",
		},
	})
	assert.NotNil(t, input.DBInstanceIdentifier)
	assert.Equal(t, true, input.ApplyImmediately)
	assert.Equal(t, int32(50), *input.MaxAllocatedStorage)
	assert.Equal(t, int32(21), *input.AllocatedStorage)
	assert.Equal(t, "db.t3.small", *input.DBInstanceClass)
}

func TestConvertSpecToModifyInput_withoutApplyImmediately(t *testing.T) {
	input := convertSpecToModifyInput(&crd.Database{
		Spec: crd.DatabaseSpec{
			MaxAllocatedSize: 50,
			Size:             21,
			ApplyImmediately: false,
			Class:            "db.t3.small",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "mydb",
			Namespace: "myns",
		},
	})
	assert.NotNil(t, input.DBInstanceIdentifier)
	assert.Equal(t, false, input.ApplyImmediately)
	assert.Equal(t, int32(50), *input.MaxAllocatedStorage)
	assert.Equal(t, int32(21), *input.AllocatedStorage)
	assert.Equal(t, "db.t3.small", *input.DBInstanceClass)
}

func TestConvertSpecToRestoreFromSnapshotInput(t *testing.T) {
	input := convertSpecToRestoreFromSnapshotInput(&crd.Database{
		Spec: crd.DatabaseSpec{
			Class:                "db.t3.small",
			DBSnapshotIdentifier: "dbidentifier",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "mydb",
			Namespace: "myns",
		},
	}, "mysubnet", []string{"sg-1234", "sg-4321"})
	assert.NotNil(t, input.DBInstanceIdentifier)
	assert.Equal(t, "db.t3.small", *input.DBInstanceClass)
	assert.Equal(t, "dbidentifier", *input.DBSnapshotIdentifier)
}
