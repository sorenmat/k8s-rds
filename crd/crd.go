package crd

import (
	"context"

	v1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

const (
	CRDPlural          string = "databases"
	CRDGroup           string = "tradeshift.com"
	CRDVersion         string = "v1"
	FullCRDName        string = "databases." + CRDGroup
	ProviderPattern    string = `local|aws`
	StorageTypePattern string = `gp2|gp3|io1`
	DBNamePattern      string = "^[A-Za-z]\\w+$"
	DBUsernamePattern  string = "^[A-Za-z]\\w+$"
)

func intptr(x int64) *int64 {
	return &x
}

func floatptr(x float64) *float64 {
	return &x
}

func NewDatabaseCRD() *apiextv1.CustomResourceDefinition {
	return &apiextv1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{Name: FullCRDName},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: CRDGroup,
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural: CRDPlural,
				Kind:   "Database",
			},
			Scope: apiextv1.NamespaceScoped,
			Conversion: &apiextv1.CustomResourceConversion{
				Strategy: "None",
			},
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    CRDVersion,
					Served:  true,
					Storage: true,
					Schema: &apiextv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]apiextv1.JSONSchemaProps{
										"provider": {
											Type:        "string",
											Description: "The provider name for this cluster. Must be local or aws.",
											Pattern:     ProviderPattern,
										},
										"username": {
											Type:        "string",
											Description: "User Name to access the database",
											MinLength:   intptr(1),
											MaxLength:   intptr(16),
											Pattern:     DBUsernamePattern,
										},
										"password": {
											Type:        "object",
											Description: "The secret name and the key name used to retrive the password for this database.",
											Required:    []string{"key", "name"},
											Properties: map[string]apiextv1.JSONSchemaProps{
												"key": {
													Type:        "string",
													Description: "The key name from the secret which contains the password used for this database.",
												},
												"name": {
													Type:        "string",
													Description: "The secret name which contains the password used for this database.",
												},
											},
										},
										"dbname": {
											Type:        "string",
											Description: "Database name",
											MinLength:   intptr(1),
											MaxLength:   intptr(63),
											Pattern:     DBNamePattern,
										},
										"engine": {
											Type:        "string",
											Description: "database engine. Ex: postgres, mysql, aurora-postgresql, etc",
										},
										"version": {
											Type:        "string",
											Description: "database engine version. ex 5.1.49",
										},
										"class": {
											Type:        "string",
											Description: "instance class name. Ex: db.m5.24xlarge or db.m3.medium",
										},
										"size": {
											Type:        "integer",
											Description: "Database size in Gb",
											Minimum:     floatptr(20),
											Maximum:     floatptr(64000),
										},
										"MaxAllocatedSize": {
											Type:        "integer",
											Description: "Database size in Gb",
											Minimum:     floatptr(20),
											Maximum:     floatptr(64000),
										},
										"multiaz": {
											Type:        "boolean",
											Description: "should it be available in multiple regions?",
										},
										"publiclyaccessible": {
											Type:        "boolean",
											Description: "is the database publicly accessible?",
										},
										"encrypted": {
											Type:        "boolean",
											Description: "should the storage be encrypted?",
										},
										"storagetype": {
											Type:        "string",
											Description: "gp2 (General Purpose SSD), gp3 (Next generation of General Purpose SSD) or io1 (Provisioned IOPS SSD)",
											Pattern:     StorageTypePattern,
										},
										"iops": {
											Type:        "integer",
											Description: "I/O operations per second",
											Minimum:     floatptr(1000),
											Maximum:     floatptr(80000),
										},
										"backupretentionperiod": {
											Type:        "integer",
											Description: "Retention period in days. 0 means disabled, 7 is the default and 35 is the maximum",
											Minimum:     floatptr(0),
											Maximum:     floatptr(35),
										},
										"deleteprotection": {
											Type:        "boolean",
											Description: "Enable or disable deletion protection",
										},
										"tags": {
											Type:        "string",
											Description: "Tags to create on the database instance format key=value,key1=value1",
										},
									},
								},
								"status": {
									Type:        "object",
									Description: "This field is added by k8s-rds operator in order to have the status of the underlying database.",
									Properties: map[string]apiextv1.JSONSchemaProps{
										"message": {
											Type:        "string",
											Description: "Provides details about creation of underlying database.",
										},
										"state": {
											Type:        "string",
											Description: "The state of the underlying database like Created/Failed.",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// CreateCRD creates the CRD resource, ignore error if it already exists
func CreateCRD(clientset apiextcs.Interface) error {
	ctx := context.Background()
	crd := NewDatabaseCRD()
	_, err := clientset.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, meta_v1.CreateOptions{})
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
	Engine                string               `json:"engine"`           // "postgres"
	Version               string               `json:"version"`          // version of the engine / database
	Class                 string               `json:"class"`            // like "db.t2.micro"
	Size                  int64                `json:"size"`             // size in gb
	MaxAllocatedSize      int64                `json:"MaxAllocatedSize"` // size in gb
	MultiAZ               bool                 `json:"multiaz,omitempty"`
	PubliclyAccessible    bool                 `json:"publicaccess,omitempty"`
	StorageEncrypted      bool                 `json:"encrypted,omitempty"`
	StorageType           string               `json:"storagetype,omitempty"`
	Iops                  int64                `json:"iops,omitempty"`
	BackupRetentionPeriod int64                `json:"backupretentionperiod,omitempty"` // between 0 and 35, zero means disable
	DeleteProtection      bool                 `json:"deleteprotection,omitempty"`
	Tags                  string               `json:"tags,omitempty"`     // key=value,key1=value1
	Provider              string               `json:"provider,omitempty"` // local or aws

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

// NewClient Creates a Rest client with the new CRD Schema
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
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{
		CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, nil, err
	}
	return client, scheme, nil
}
