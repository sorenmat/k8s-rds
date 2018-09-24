package dbprovider

import (
	"github.com/sorenmat/k8s-rds/crd"
	"k8s.io/client-go/kubernetes"
)

type ContainerDBProvider struct {
	kubectl *kubernetes.Clientset
}

func (provider ContainerDBProvider) CreateDatabase(kubectl *kubernetes.Clientset, db *crd.Database) (string,error) {
	return "",nil
}

func (provider ContainerDBProvider) DeleteDatabase(kubectl *kubernetes.Clientset, db *crd.Database) (error) {
	return nil
}
