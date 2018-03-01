package rds

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/sorenmat/k8s-rds/crd"
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
	i := convertSpecToInput(db, "mysubnet", "mypassword")
	assert.Equal(t, "mydb", *i.DBName)
	assert.Equal(t, "postgres", *i.Engine)
	assert.Equal(t, "mypassword", *i.MasterUserPassword)
	assert.Equal(t, "myuser", *i.MasterUsername)
	assert.Equal(t, "db.t2.micro", *i.DBInstanceClass)
	assert.Equal(t, int64(100), *i.AllocatedStorage)
	assert.Equal(t, true, *i.PubliclyAccessible)
	assert.Equal(t, true, *i.MultiAZ)
	assert.Equal(t, true, *i.StorageEncrypted)
	assert.Equal(t, "bad", *i.StorageType)
	assert.Equal(t, int64(1000), *i.Iops)
}

type mockedReceiveMsgs struct {
}

func TestGetEndpoints(t *testing.T) {
	c := client.New(aws.Config{}, metadata.ClientInfo{}, request.Handlers{})
	c.Handlers.Clear()
	c.Handlers.Send.PushBack(func(r *request.Request) {
		data := r.Data.(*rds.DescribeDBInstancesOutput)
		data.DBInstances = append(data.DBInstances, &rds.DBInstance{Endpoint: &rds.Endpoint{Address: aws.String("https://something.com")}})
	})

	svc := &rds.RDS{Client: c}
	name, err := getEndpoint("name", svc)
	assert.NoError(t, err)
	assert.NotNil(t, name)
	assert.Equal(t, "https://something.com", name)
}
func TestGetEndpointsFailure(t *testing.T) {
	c := client.New(aws.Config{}, metadata.ClientInfo{}, request.Handlers{})
	c.Handlers.Clear()
	c.Handlers.Send.PushBack(func(r *request.Request) {
		data := r.Data.(*rds.DescribeDBInstancesOutput)
		data.DBInstances = nil
	})

	svc := &rds.RDS{Client: c}
	_, err := getEndpoint("name", svc)
	assert.Error(t, err)
}
