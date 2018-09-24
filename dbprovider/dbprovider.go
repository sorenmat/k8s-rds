package dbprovider

import (
	"github.com/sorenmat/k8s-rds/crd"
	"k8s.io/client-go/kubernetes"
)

type DBProvider interface {
	CreateDatabase(kubectl *kubernetes.Clientset, db *crd.Database) (string,error)
	DeleteDatabase(kubectl *kubernetes.Clientset, db *crd.Database) (error)
}

