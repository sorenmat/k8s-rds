package local

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
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
			DBName:             aws.String("mydb"),
			Engine:             "postgres",
			Username:           "myuser",
			Class:              "db.t2.micro",
			Size:               aws.Int64(100),
			MultiAZ:            true,
			PubliclyAccessible: true,
			StorageEncrypted:   true,
			StorageType:        "bad",
			Iops:               1000,
			Password:           v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	repository := "registry.bwtsi.cn"
	spec := toSpec(db, repository)
	assert.Equal(t, "mydb", spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "registry.bwtsi.cn/postgres:latest", spec.Template.Spec.Containers[0].Image)
}

func TestCreateDatabase(t *testing.T) {
	db := &crd.Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "mydb"},
		Spec: crd.DatabaseSpec{
			DBName:             aws.String("mydb"),
			Engine:             "postgres",
			Username:           "myuser",
			Class:              "db.t2.micro",
			Size:               aws.Int64(100),
			MultiAZ:            true,
			PubliclyAccessible: true,
			StorageEncrypted:   true,
			StorageType:        "bad",
			Iops:               1000,
			Password:           v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	kc := testclient.NewSimpleClientset()
	repository := ""
	l, err := New(kc, repository)
	assert.NoError(t, err)
	// we need it to not wait for status
	l.SkipWaiting = true
	host, err := l.CreateDatabase(context.Background(), db)
	assert.NoError(t, err)
	assert.NotEmpty(t, host)

	sequence := []struct {
		Action   string
		Group    string
		Resource string
	}{
		{
			Action:   "get",
			Group:    "",
			Resource: "persistentvolumeclaims",
		},
		{
			Action:   "create",
			Group:    "",
			Resource: "persistentvolumeclaims",
		},
		{
			Action:   "get",
			Group:    "apps",
			Resource: "deployments",
		},
		{
			Action:   "create",
			Group:    "apps",
			Resource: "deployments",
		},
	}

	for i, action := range kc.Fake.Actions() {
		assert.Equal(t, sequence[i].Action, action.GetVerb())
		assert.Equal(t, sequence[i].Group, action.GetResource().GroupResource().Group)
		assert.Equal(t, sequence[i].Resource, action.GetResource().GroupResource().Resource)
	}

}

func TestUpdateDatabase(t *testing.T) {
	db := &crd.Database{
		ObjectMeta: meta_v1.ObjectMeta{Name: "mydb"},
		Spec: crd.DatabaseSpec{
			DBName:             aws.String("mydb"),
			Engine:             "postgres",
			Username:           "myuser",
			Class:              "db.t2.micro",
			Size:               aws.Int64(100),
			MultiAZ:            true,
			PubliclyAccessible: true,
			StorageEncrypted:   true,
			StorageType:        "bad",
			Iops:               1000,
			Password:           v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "password"}, Key: "mypassword"},
		},
	}
	kc := testclient.NewSimpleClientset()
	repository := ""
	l, err := New(kc, repository)
	assert.NoError(t, err)
	// we need it to not wait for status
	l.SkipWaiting = true
	host, err := l.CreateDatabase(context.Background(), db)
	assert.NoError(t, err)
	assert.NotEmpty(t, host)
	assert.Equal(t, 4, len(kc.Fake.Actions()))
	_, err = l.CreateDatabase(context.Background(), db)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(kc.Fake.Actions()))

	sequence := []struct {
		Action   string
		Group    string
		Resource string
	}{
		{
			Action:   "get",
			Group:    "",
			Resource: "persistentvolumeclaims",
		},
		{
			Action:   "create",
			Group:    "",
			Resource: "persistentvolumeclaims",
		},
		{
			Action:   "get",
			Group:    "apps",
			Resource: "deployments",
		},
		{
			Action:   "create",
			Group:    "apps",
			Resource: "deployments",
		},

		{
			Action:   "get",
			Group:    "",
			Resource: "persistentvolumeclaims",
		},
		{
			Action:   "get",
			Group:    "",
			Resource: "persistentvolumeclaims",
		},
		{
			Action:   "get",
			Group:    "apps",
			Resource: "deployments",
		},
		{
			Action:   "update",
			Group:    "apps",
			Resource: "deployments",
		},
	}

	for i, action := range kc.Fake.Actions() {
		assert.Equal(t, sequence[i].Action, action.GetVerb())
		assert.Equal(t, sequence[i].Group, action.GetResource().GroupResource().Group)
		assert.Equal(t, sequence[i].Resource, action.GetResource().GroupResource().Resource)
	}
}
