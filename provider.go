package main

import (
	"github.com/sorenmat/k8s-rds/client"
	"github.com/sorenmat/k8s-rds/crd"
)

// DatabaseProvider is the interface for creating and deleting databases
// this is the main interface that should be implemented if a new provider is created
type DatabaseProvider interface {
	CreateDatabase(*crd.Database, *client.Crdclient) error
	DeleteDatabase(*crd.Database) error
}
