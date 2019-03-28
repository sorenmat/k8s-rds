package client

import (
	"github.com/cloud104/k8s-rds/crd"

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

// Crdclient ...
type Crdclient struct {
	cl     *rest.RESTClient
	ns     string
	plural string
	codec  runtime.ParameterCodec
}

// Create ...
func (f *Crdclient) Create(obj *crd.Database) (*crd.Database, error) {
	var result crd.Database
	err := f.cl.Post().Namespace(f.ns).Resource(f.plural).Body(obj).Do().Into(&result)
	return &result, err
}

// Update ...
func (f *Crdclient) Update(obj *crd.Database) (*crd.Database, error) {
	var result crd.Database

	err := f.cl.Put().Namespace(f.ns).Resource(f.plural).Name(obj.Name).Body(obj).Do().Into(&result)
	return &result, err
}

// Delete ...
func (f *Crdclient) Delete(name string, options *meta_v1.DeleteOptions) error {
	return f.cl.Delete().
		Namespace(f.ns).Resource(f.plural).Name(name).Body(options).Do().
		Error()
}

// Get ...
func (f *Crdclient) Get(name string) (*crd.Database, error) {
	var result crd.Database
	err := f.cl.Get().Namespace(f.ns).Resource(f.plural).Name(name).Do().Into(&result)
	return &result, err
}

// List ...
func (f *Crdclient) List(opts meta_v1.ListOptions) (*crd.DatabaseList, error) {
	var result crd.DatabaseList
	err := f.cl.Get().Namespace(f.ns).Resource(f.plural).VersionedParams(&opts, f.codec).Do().Into(&result)
	return &result, err
}

// Create a new List watch for our CRD
// NewListWatch ...
func (f *Crdclient) NewListWatch() *cache.ListWatch {
	return cache.NewListWatchFromClient(f.cl, f.plural, f.ns, fields.Everything())
}
