package local

import (
	"testing"

	"github.com/sorenmat/k8s-rds/crd"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestConvertSpecToDeployment(t *testing.T) {
	db := &crd.Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "mydb"},
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
	spec := toSpec(db)
	assert.Equal(t, "mydb", spec.Template.Spec.Containers[0].Name)
}

func TestCreateDatabase(t *testing.T) {
	db := &crd.Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "mydb"},
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
	kc := testclient.NewSimpleClientset()
	l, err := New(db, kc)
	assert.NoError(t, err)
	host, err := l.CreateDatabase(db)
	assert.NoError(t, err)
	assert.NotEmpty(t, host)

	assert.Equal(t, "get", kc.Fake.Actions()[0].GetVerb())
	assert.Equal(t, "apps", kc.Fake.Actions()[0].GetResource().GroupResource().Group)
	assert.Equal(t, "deployments", kc.Fake.Actions()[0].GetResource().GroupResource().Resource)
	// create it
	assert.Equal(t, "create", kc.Fake.Actions()[1].GetVerb())
	assert.Equal(t, "apps", kc.Fake.Actions()[1].GetResource().GroupResource().Group)
	assert.Equal(t, "deployments", kc.Fake.Actions()[1].GetResource().GroupResource().Resource)
}

func TestUpdateDatabase(t *testing.T) {
	db := &crd.Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "mydb"},
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
	kc := testclient.NewSimpleClientset()
	l, err := New(db, kc)
	assert.NoError(t, err)
	host, err := l.CreateDatabase(db)
	assert.NoError(t, err)
	assert.NotEmpty(t, host)
	assert.Equal(t, 2, len(kc.Fake.Actions()))
	host, err = l.CreateDatabase(db)

	assert.Equal(t, 4, len(kc.Fake.Actions()))
	assert.Equal(t, "get", kc.Fake.Actions()[2].GetVerb())
	assert.Equal(t, "apps", kc.Fake.Actions()[2].GetResource().GroupResource().Group)
	assert.Equal(t, "deployments", kc.Fake.Actions()[2].GetResource().GroupResource().Resource)
	// create it
	assert.Equal(t, "update", kc.Fake.Actions()[3].GetVerb())
	assert.Equal(t, "apps", kc.Fake.Actions()[3].GetResource().GroupResource().Group)
	assert.Equal(t, "deployments", kc.Fake.Actions()[3].GetResource().GroupResource().Resource)
}
