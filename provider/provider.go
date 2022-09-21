package provider

import (
	"context"

	"github.com/sorenmat/k8s-rds/crd"
)

// DatabaseProvider is the interface for creating and deleting databases
// this is the main interface that should be implemented if a new provider is created
type DatabaseProvider interface {
	CreateDatabase(context.Context, *crd.Database) (string, error)
	UpdateDatabase(context.Context, *crd.Database) error
	DeleteDatabase(context.Context, *crd.Database) error
	CreateDBCluster(context.Context, *crd.DBCluster) (string, error)
	UpdateDBCluster(context.Context, *crd.DBCluster) error
	DeleteDBCluster(context.Context, *crd.DBCluster) error
	ServiceProvider
}

type ServiceProvider interface {
	CreateService(ctx context.Context, namespace string, hostname string, internalname string) error
	DeleteService(ctx context.Context, namespace string, dbname string) error
	GetSecret(ctx context.Context, namepspace string, pwname string, pwkey string) (string, error)
}
