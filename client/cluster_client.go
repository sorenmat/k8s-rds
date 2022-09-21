package client

import (
	"context"
	"log"

	"github.com/sorenmat/k8s-rds/crd"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// This file implement all the (CRUD) client methods we need to access our CRD object

func DBClusterCrdClient(cl *rest.RESTClient, scheme *runtime.Scheme, namespace string) *DBClusterCrdclient {
	return &DBClusterCrdclient{cl: cl, ns: namespace, plural: "dbclusters",
		codec: runtime.NewParameterCodec(scheme)}
}

type DBClusterCrdclient struct {
	cl     *rest.RESTClient
	ns     string
	plural string
	codec  runtime.ParameterCodec
}

func (f *DBClusterCrdclient) Create(ctx context.Context, obj *crd.DBCluster) (*crd.DBCluster, error) {
	var result crd.DBCluster
	err := f.cl.Post().
		Namespace(f.ns).Resource(f.plural).
		Body(obj).Do(ctx).Into(&result)
	return &result, err
}

func (f *DBClusterCrdclient) Update(ctx context.Context, obj *crd.DBCluster) (*crd.DBCluster, error) {
	var result crd.DBCluster
	err := f.cl.Put().
		Namespace(f.ns).Resource(f.plural).Name(obj.Name).
		Body(obj).Do(ctx).Into(&result)
	log.Printf("New resource version of the DB cluster %s is %s\n", result.Name,
		result.ResourceVersion)
	return &result, err
}

func (f *DBClusterCrdclient) Delete(ctx context.Context, name string, options *meta_v1.DeleteOptions) error {
	return f.cl.Delete().
		Namespace(f.ns).Resource(f.plural).
		Name(name).Body(options).Do(ctx).
		Error()
}

func (f *DBClusterCrdclient) Get(ctx context.Context, name string) (*crd.DBCluster, error) {
	var result crd.DBCluster
	err := f.cl.Get().
		Namespace(f.ns).Resource(f.plural).
		Name(name).Do(ctx).Into(&result)
	return &result, err
}

func (f *DBClusterCrdclient) List(ctx context.Context, opts meta_v1.ListOptions) (*crd.DBClusterList, error) {
	var result crd.DBClusterList
	err := f.cl.Get().
		Namespace(f.ns).Resource(f.plural).
		VersionedParams(&opts, f.codec).
		Do(ctx).Into(&result)
	return &result, err
}

// Create a new List watch for our CRD
func (f *DBClusterCrdclient) NewListWatch() *cache.ListWatch {
	return cache.NewListWatchFromClient(f.cl, f.plural, f.ns, fields.Everything())
}
