package local

import (
	"fmt"
	"log"

	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/kube"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// create an External named service object for Kubernetes
func (l *Local) createServiceObj(s *v1.Service, namespace string, hostname string, internalname string) *v1.Service {
	var ports []v1.ServicePort

	ports = append(ports, v1.ServicePort{
		Name:       "pgsql",
		Port:       int32(5432),
		TargetPort: intstr.IntOrString{IntVal: int32(5432)},
	})
	s.Spec.Type = "ClusterIP"

	s.Spec.Ports = ports
	s.Name = internalname
	s.Spec.Selector = map[string]string{"db": internalname}
	s.Annotations = map[string]string{"origin": "k8s-rds"}
	s.Namespace = namespace
	return s
}

// CreateService Creates or updates a service in Kubernetes with the new information
func (l *Local) CreateService(namespace string, hostname string, internalname string) error {
	client, err := kube.Client()
	if err != nil {
		return err
	}
	// create a service in kubernetes that points to the AWS RDS instance
	serviceInterface := client.CoreV1().Services(namespace)

	s, sErr := serviceInterface.Get(hostname, metav1.GetOptions{})

	create := false
	if sErr != nil {
		s = &v1.Service{}
		create = true
	}
	s = l.createServiceObj(s, namespace, hostname, internalname)

	if create {
		_, err = serviceInterface.Create(s)
	} else {
		_, err = serviceInterface.Update(s)
	}

	return err
}

func (l *Local) DeleteService(namespace string, dbname string) error {
	client, err := kube.Client()
	if err != nil {
		return err
	}
	serviceInterface := client.CoreV1().Services(namespace)
	err = serviceInterface.Delete(dbname, &metav1.DeleteOptions{})
	if err != nil {
		log.Println(err)
		return errors.Wrap(err, fmt.Sprintf("delete of service %v failed in namespace %v", dbname, namespace))
	}
	return nil
}

func (l *Local) GetSecret(namespace string, name string, key string) (string, error) {
	client, err := kube.Client()
	if err != nil {
		return "", err
	}
	secret, err := client.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("unable to fetch secret %v", name))
	}
	password := secret.Data[key]
	return string(password), nil
}
