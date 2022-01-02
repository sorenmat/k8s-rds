package kube

import (
	"context"
	"fmt"
	"log"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

type Kube struct {
	Client *kubernetes.Clientset
}

// create an External named service object for Kubernetes
func (k *Kube) createServiceObj(s *v1.Service, namespace string, hostname string, internalname string) *v1.Service {
	var ports []v1.ServicePort

	ports = append(ports, v1.ServicePort{
		Name:       "pgsql",
		Port:       int32(5432),
		TargetPort: intstr.IntOrString{IntVal: int32(5432)},
	})
	s.Spec.Type = "ExternalName"
	s.Spec.ExternalName = hostname

	s.Spec.Ports = ports
	s.Name = internalname
	s.Annotations = map[string]string{"origin": "rds"}
	s.Namespace = namespace
	return s
}

// CreateService Creates or updates a service in Kubernetes with the new information
func (k *Kube) CreateService(ctx context.Context, namespace string, hostname string, internalname string) error {
	// create a service in kubernetes that points to the AWS RDS instance
	serviceInterface := k.Client.CoreV1().Services(namespace)

	s, sErr := serviceInterface.Get(ctx, hostname, metav1.GetOptions{})

	create := false
	if sErr != nil {
		s = &v1.Service{}
		create = true
	}
	s = k.createServiceObj(s, namespace, hostname, internalname)
	var err error
	if create {
		_, err = serviceInterface.Create(ctx, s, metav1.CreateOptions{})
	} else {
		_, err = serviceInterface.Update(ctx, s, metav1.UpdateOptions{})
	}

	return err
}

func (k *Kube) DeleteService(ctx context.Context, namespace string, dbname string) error {
	serviceInterface := k.Client.CoreV1().Services(namespace)
	err := serviceInterface.Delete(ctx, dbname, metav1.DeleteOptions{})
	if err != nil {
		log.Println(err)
		return errors.Wrap(err, fmt.Sprintf("delete of service %v failed in namespace %v", dbname, namespace))
	}
	return nil
}

func (k *Kube) GetSecret(ctx context.Context, namespace string, name string, key string) (string, error) {
	secret, err := k.Client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("unable to fetch secret %v", name))
	}
	password := secret.Data[key]
	return string(password), nil
}
