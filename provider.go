package main

import (
	"github.com/cloud104/k8s-rds/client"
	"github.com/cloud104/k8s-rds/crd"
	"k8s.io/api/core/v1"
)

// DatabaseProvider is the interface for creating and deleting databases
// this is the main interface that should be implemented if a new provider is created
type DatabaseProvider interface {
	CreateDatabase(*crd.Database, *client.Crdclient, string) (string, error)
	DeleteDatabase(*crd.Database) error
}

type ServiceProvider interface {
	CreateService(s *v1.Service, namespace string, hostname string, internalname string) *v1.Service
	DeleteService(namespace string, dbname string) error
}
