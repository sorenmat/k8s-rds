package crd

import (
	"k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

const (
	CRDPlural   string = "databases"
	CRDGroup    string = "k8s.io"
	CRDVersion  string = "v1"
	FullCRDName string = "databases." + CRDGroup
)

// CreateCRD creates the CRD resource, ignore error if it already exists
func CreateCRD(clientset apiextcs.Interface) error {
	crd := &apiextv1beta1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{Name: FullCRDName},
		Spec: apiextv1beta1.CustomResourceDefinitionSpec{
			Group:   CRDGroup,
			Version: CRDVersion,
			Scope:   apiextv1beta1.NamespaceScoped,
			Names: apiextv1beta1.CustomResourceDefinitionNames{
				Plural: "databases",
				Kind:   "Database",
			},
		},
	}

	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil && apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// Database is the definition of our CRD Database
type Database struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               DatabaseSpec   `json:"spec"`
	Status             DatabaseStatus `json:"status,omitempty"`
}

// DatabaseSpec main structure describing the database instance
type DatabaseSpec struct {
	Username              string               `json:"username"`
	Password              v1.SecretKeySelector `json:"password"`
	DBName                string               `json:"dbname"`
	DBSnapshotIdentifier  string               `json:"dbsnapshotidentifier,omitempty"` // "rds:...." will trigger a restore snapshot
	Engine                string               `json:"engine"`                         // "postgres"
	Class                 string               `json:"class"`                          // like "db.t2.micro"
	Size                  int64                `json:"size"`                           // size in gb
	MultiAZ               bool                 `json:"multiaz,omitempty"`
	PubliclyAccessible    bool                 `json:"publicaccess,omitempty"`
	SecurityGroups        []string             `json:"securitygroups"`
	Subnets               []string             `json:"subnets"`
	DBSubnetGroupName     string               `json:"subnetgroup,omitempty"`    // DB subnet group name
	DBParameterGroupName  string               `json:"parametergroup,omitempty"` // DB parameter group name
	StorageEncrypted      bool                 `json:"encrypted,omitempty"`
	StorageType           string               `json:"storagetype,omitempty"`
	Iops                  int64                `json:"iops,omitempty"`
	BackupRetentionPeriod *int64               `json:"backupretentionperiod,omitempty"` // between 0 and 35, zero means disable
}

type DatabaseStatus struct {
	State   string `json:"state,omitempty" description:"State of the deploy"`
	Message string `json:"message,omitempty" description:"Detailed message around the state"`
}

type DatabaseList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`
	Items            []Database `json:"items"`
}

func (d *Database) DeepCopyObject() runtime.Object {
	return d
}

func (d *DatabaseList) DeepCopyObject() runtime.Object {
	return d
}

var SchemeGroupVersion = schema.GroupVersion{Group: CRDGroup, Version: CRDVersion}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Database{},
		&DatabaseList{},
	)
	meta_v1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// Create a Rest client with the new CRD Schema
func NewClient(cfg *rest.Config) (*rest.RESTClient, *runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	SchemeBuilder := runtime.NewSchemeBuilder(addKnownTypes)
	if err := SchemeBuilder.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}
	config := *cfg
	config.GroupVersion = &SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{
		CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, nil, err
	}
	return client, scheme, nil
}
