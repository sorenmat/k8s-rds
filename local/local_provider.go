package local

import (
	"fmt"
	"log"

	"github.com/sorenmat/k8s-rds/crd"
	"github.com/sorenmat/k8s-rds/provider"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Local struct {
	ServiceProvider provider.ServiceProvider
	kc              kubernetes.Interface
}

func New(db *crd.Database, kc kubernetes.Interface) (*Local, error) {
	r := Local{kc: kc}
	return &r, nil
}

// CreateDatabase creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *Local) CreateDatabase(db *crd.Database) (string, error) {
	new := false
	d, err := r.kc.AppsV1().Deployments(db.Namespace).Get(db.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		// we got an error and it's not the NotFound, let's crash
		return "", err
	}
	if errors.IsNotFound(err) {
		// Deployment seems to be empty, let's assume it means we need to create it
		d = &v1.Deployment{}
		new = true
	}

	d.Name = db.Name
	d.Labels = map[string]string{"db": "true"}

	d.ObjectMeta = metav1.ObjectMeta{
		Name: db.Name,
	}
	d.Spec = toSpec(db)

	if new {
		log.Printf("creating database %v", db.Name)
		_, err = r.kc.AppsV1().Deployments(db.Namespace).Create(d)
		if err != nil {
			return "", err
		}
	} else {
		log.Printf("updating database %v", db.Name)
		_, err = r.kc.AppsV1().Deployments(db.Namespace).Update(d)
		if err != nil {
			return "", err
		}
	}

	return db.Name, nil
}

func (r *Local) DeleteDatabase(db *crd.Database) error {
	// delete the database instance
	err := r.kc.AppsV1().Deployments(db.Namespace).Delete(db.Name, &metav1.DeleteOptions{})
	return err
}

func int32Ptr(i int32) *int32 { return &i }

func toSpec(db *crd.Database) v1.DeploymentSpec {
	fmt.Println("Key:", db.Spec.Password.Key)
	return v1.DeploymentSpec{
		Replicas: int32Ptr(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"db": db.Name,
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"db": db.Name,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  db.Name,
						Image: db.Spec.Engine, // TODO is this correct
						Env: []corev1.EnvVar{corev1.EnvVar{
							Name: "POSTGRES_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: db.Spec.Password.Name}, Key: db.Spec.Password.Key}}},
							corev1.EnvVar{Name: "POSTGRES_USER", Value: db.Spec.Username},
							corev1.EnvVar{Name: "POSTGRES_DB", Value: db.Spec.DBName},
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "pgsql",
								Protocol:      corev1.ProtocolTCP,
								ContainerPort: 5432,
							},
						}},
				},
			},
		},
	}
}
