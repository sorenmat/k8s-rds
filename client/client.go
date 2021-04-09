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

func CrdClient(cl *rest.RESTClient, scheme *runtime.Scheme, namespace string) *Crdclient {
	return &Crdclient{cl: cl, ns: namespace, plural: "databases",
		codec: runtime.NewParameterCodec(scheme)}
}

type Crdclient struct {
	cl     *rest.RESTClient
	ns     string
	plural string
	codec  runtime.ParameterCodec
}

func (f *Crdclient) Create(ctx context.Context, obj *crd.Database) (*crd.Database, error) {
	var result crd.Database
	err := f.cl.Post().
		Namespace(f.ns).Resource(f.plural).
		Body(obj).Do(ctx).Into(&result)
	return &result, err
}

func (f *Crdclient) Update(ctx context.Context, obj *crd.Database) (*crd.Database, error) {
	var result crd.Database
	err := f.cl.Put().
		Namespace(f.ns).Resource(f.plural).Name(obj.Name).
		Body(obj).Do(ctx).Into(&result)
	log.Printf("New resource version of the DB %s is %s\n", result.Name,
		result.ResourceVersion)
	return &result, err
}

func (f *Crdclient) Delete(ctx context.Context, name string, options *meta_v1.DeleteOptions) error {
	return f.cl.Delete().
		Namespace(f.ns).Resource(f.plural).
		Name(name).Body(options).Do(ctx).
		Error()
}

func (f *Crdclient) Get(ctx context.Context, name string) (*crd.Database, error) {
	var result crd.Database
	err := f.cl.Get().
		Namespace(f.ns).Resource(f.plural).
		Name(name).Do(ctx).Into(&result)
	return &result, err
}

func (f *Crdclient) List(ctx context.Context, opts meta_v1.ListOptions) (*crd.DatabaseList, error) {
	var result crd.DatabaseList
	err := f.cl.Get().
		Namespace(f.ns).Resource(f.plural).
		VersionedParams(&opts, f.codec).
		Do(ctx).Into(&result)
	return &result, err
}

// Create a new List watch for our CRD
func (f *Crdclient) NewListWatch() *cache.ListWatch {
	return cache.NewListWatchFromClient(f.cl, f.plural, f.ns, fields.Everything())
}
