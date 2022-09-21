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
	DBClusterCRDPlural   string = "dbclusters"
	DBClusterKind        string = "DBCluster"
	DBClusterCRDVersion  string = "v1"
	DBClusterFullCRDName string = "dbclusters." + CRDGroup
)

func NewDBClusterCRD() *apiextv1.CustomResourceDefinition {
	return &apiextv1.CustomResourceDefinition{
		ObjectMeta: meta_v1.ObjectMeta{Name: DBClusterFullCRDName},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: CRDGroup,
			Scope: apiextv1.NamespaceScoped,
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural:     DBClusterCRDPlural,
				Kind:       DBClusterKind,
				ShortNames: []string{"cls"},
			},
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    DBClusterCRDVersion,
					Storage: true,
					Served:  true,
					Schema: &apiextv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]apiextv1.JSONSchemaProps{
										"DBName": {
											Type:        "string",
											Description: "Database name",
											MinLength:   intptr(1),
											MaxLength:   intptr(63),
											Pattern:     DBNamePattern,
										},
										"MasterUsername": {
											Type:        "string",
											Description: "The name of the master user for the DB cluster.",
											MinLength:   intptr(1),
											MaxLength:   intptr(16),
											Pattern:     DBUsernamePattern,
										},
										"MasterUserPassword": {
											Type:        "object",
											Description: "The password for the master database user.",
											Properties: map[string]apiextv1.JSONSchemaProps{
												"name": {
													Type:        "string",
													Description: "secret name",
												},
												"key": {
													Type:        "string",
													Description: "secret key",
												},
											},
										},
										"DBClusterIdentifier": {
											Type:        "string",
											Description: "The DB cluster identifier. This parameter is stored as a lowercase string.",
										},
										"Engine": {
											Type:        "string",
											Description: "The name of the database engine to be used for this DB cluster.",
										},
										"EngineVersion": {
											Type:        "string",
											Description: "The version number of the database engine to use.",
										},
										"AllocatedStorage": {
											Type: "integer",
											Description: `The amount of storage in gibibytes (GiB) to allocate to each DB instance in the Multi-AZ DB cluster.
											This setting is required to create a Multi-AZ DB cluster.
											Valid for: Multi-AZ DB clusters only.`,
										},
										"BackupRetentionPeriod": {
											Type:        "integer",
											Description: "The number of days for which automated backups are retained.",
											Minimum:     floatptr(0),
											Maximum:     floatptr(35),
										},
										"DBClusterInstanceClass": {
											Type: "string",
											Description: `The compute and memory capacity of each DB instance in the Multi-AZ DB cluster,
											for example db.m6g.xlarge. Not all DB instance classes are available in all
											Amazon Web Services Regions, or for all database engines.`,
										},
										"DeletionProtection": {
											Type:        "boolean",
											Description: "A value that indicates whether the DB cluster has deletion protection enabled.",
										},
										"Iops": {
											Type:        "integer",
											Description: "The amount of Provisioned IOPS (input/output operations per second) to be initially allocated for each DB instance in the Multi-AZ DB cluster.",
											Minimum:     floatptr(1000),
											Maximum:     floatptr(80000),
										},
										"Port": {
											Type:        "integer",
											Description: "The port number on which the instances in the DB cluster accept connections.",
											Minimum:     floatptr(1150),
											Maximum:     floatptr(65535),
										},
										"StorageType": {
											Type:        "string",
											Description: "gp2 (General Purpose SSD) or io1 (Provisioned IOPS SSD)",
											Pattern:     StorageTypePattern,
										},
										"Provider": {
											Type:        "string",
											Description: "Type of provider (aws, local) (default \"aws\")",
											Pattern:     "aws|local",
										},
										"Tags": {
											Type:        "string",
											Description: "Tags to create on the cluster instance format key=value,key1=value1",
										},
										"StorageEncrypted": {
											Type:        "boolean",
											Description: "should the storage be encrypted?",
										},
										"MultiAZ": {
											Type:        "boolean",
											Description: "should it be available in multiple regions?",
										},
										"PubliclyAccessible": {
											Type:        "boolean",
											Description: "is the database publicly accessible?",
										},
										"SkipFinalSnapshot": {
											Type:        "boolean",
											Description: "Indicates whether to skip the creation of a final DB snapshot before deleting the instance. By default, skipfinalsnapshot isn't enabled, and the DB snapshot is created.",
										},
										"ApplyImmediately": {
											Type:        "boolean",
											Description: "When you modify a DB instance, you can apply the changes immediately by setting the ApplyImmediately parameter to true. If you don't choose to apply changes immediately, the changes are put into the pending modifications queue. During the next maintenance window, any pending changes in the queue are applied. If you choose to apply changes immediately, your new changes and any changes in the pending modifications queue are applied. ",
										},
										"SnapshotIdentifier": {
											Type:        "string",
											Description: "Cluster snapshot identifier to restore from.",
										},
										"ServerlessV2ScalingConfiguration": {
											Type: "object",
											Properties: map[string]apiextv1.JSONSchemaProps{
												"MinCapacity": {
													Type:        "number",
													Description: "minimum capacity value for each Aurora Serverless v2 writer or reader.",
													Minimum:     floatptr(0.5),
													Maximum:     floatptr(128),
												},
												"MaxCapacity": {
													Type:        "number",
													Description: "Maximum capacity value that each Aurora Serverless v2 writer or reader can scale to.",
													Minimum:     floatptr(0.5),
													Maximum:     floatptr(128),
												},
											},
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
func CreateDBClusterCRD(clientset apiextcs.Interface) error {
	ctx := context.Background()
	crd := NewDBClusterCRD()
	_, err := clientset.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, meta_v1.CreateOptions{})
	if err != nil && apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// Database is the definition of our CRD Database
type DBCluster struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               DBClusterSpec   `json:"spec"`
	Status             DBClusterStatus `json:"status,omitempty"`
}
type ServerlessV2ScalingConfiguration struct {
	MaxCapacity *float64 `json:"MaxCapacity"`
	MinCapacity *float64 `json:"MinCapacity"`
}

// DBClusterSpec main structure describing the database cluster
type DBClusterSpec struct {
	DBName                           *string                           `json:"DBName"`
	MasterUsername                   string                            `json:"MasterUsername"`
	MasterUserPassword               v1.SecretKeySelector              `json:"MasterUserPassword"`
	DBClusterIdentifier              string                            `json:"DBClusterIdentifier"`
	Engine                           string                            `json:"Engine"`
	EngineVersion                    string                            `json:"EngineVersion"`
	AllocatedStorage                 int64                             `json:"AllocatedStorage"`
	BackupRetentionPeriod            int64                             `json:"BackupRetentionPeriod,omitempty"`
	DBClusterInstanceClass           string                            `json:"DBClusterInstanceClass"`
	DeletionProtection               bool                              `json:"DeletionProtection,omitempty"`
	Iops                             int64                             `json:"Iops,omitempty"`
	Port                             int64                             `json:"Port"`
	StorageType                      string                            `json:"storagetype,omitempty"`
	Provider                         string                            `json:"provider,omitempty"` // local or aws
	Tags                             string                            `json:"tags,omitempty"`     // key=value,key1=value1
	StorageEncrypted                 bool                              `json:"encrypted,omitempty"`
	ServerlessV2ScalingConfiguration *ServerlessV2ScalingConfiguration `json:"ServerlessV2ScalingConfiguration,omitempty"`
	MultiAZ                          bool                              `json:"MultiAZ,omitempty"`
	SkipFinalSnapshot                bool                              `json:"SkipFinalSnapshot,omitempty"`
	PubliclyAccessible               *bool                             `json:"PubliclyAccessible,omitempty"`
	ApplyImmediately                 bool                              `json:"ApplyImmediately,omitempty"`
	SnapshotIdentifier               string                            `json:"SnapshotIdentifier,omitempty"`
}

type DBClusterStatus struct {
	State   string `json:"state,omitempty" description:"State of the deploy"`
	Message string `json:"message,omitempty" description:"Detailed message around the state"`
}

type DBClusterList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`
	Items            []DBCluster `json:"items"`
}

func (d *DBCluster) DeepCopyObject() runtime.Object {
	return d
}

func (d *DBClusterList) DeepCopyObject() runtime.Object {
	return d
}

var DBClusterSchemeGroupVersion = schema.GroupVersion{Group: CRDGroup, Version: DBClusterCRDVersion}

func addDBClusterKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(DBClusterSchemeGroupVersion,
		&DBCluster{},
		&DBClusterList{},
	)
	meta_v1.AddToGroupVersion(scheme, DBClusterSchemeGroupVersion)
	return nil
}

// NewClient Creates a Rest client with the new CRD Schema
func NewDBClusterClient(cfg *rest.Config) (*rest.RESTClient, *runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	SchemeBuilder := runtime.NewSchemeBuilder(addDBClusterKnownTypes)
	if err := SchemeBuilder.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}
	config := *cfg
	config.GroupVersion = &DBClusterSchemeGroupVersion
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
