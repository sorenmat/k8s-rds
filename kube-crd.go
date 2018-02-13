/*
Copyright 2016 Iguazio Systems Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/sorenmat/cool-aide/client"
	"github.com/sorenmat/cool-aide/crd"

	"flag"

	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// return rest config, if path not specified assume in cluster config
func GetClientConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func main() {
	//clientset := getKubectl()
	kubeconf := flag.String("kubeconf", "admin.conf", "Path to a kube config. Only required if out-of-cluster.")
	flag.Parse()

	config, err := GetClientConfig(*kubeconf)
	if err != nil {
		panic(err.Error())
	}

	// create clientset and create our CRD, this only need to run once
	clientset, err := apiextcs.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// note: if the CRD exist our CreateCRD function is set to exit without an error
	err = crd.CreateCRD(clientset)
	if err != nil {
		panic(err)
	}

	// Wait for the CRD to be created before we use it (only needed if its a new one)
	time.Sleep(3 * time.Second)

	// Create a new clientset which include our CRD schema
	crdcs, scheme, err := crd.NewClient(config)
	if err != nil {
		panic(err)
	}

	// Create a CRD client interface
	crdclient := client.CrdClient(crdcs, scheme, "default")

	// Create a new Example object and write to k8s
	example := &crd.Database{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   "gabby-db",
			Labels: map[string]string{"database_type": "RDS"},
		},
		Spec: crd.DatabaseSpec{
			Class:    "db.t2.micro",
			DBName:   "gabby-db",
			Name:     "gabby-db",
			Username: "gabby",
			Password: "gabbydatabasepasswordDxxCEEc",
		},
		Status: crd.DatabaseStatus{
			State:   "created",
			Message: "Created, not processed yet",
		},
	}

	_, err = crdclient.Create(example)
	if err == nil {
		log.Println("Database CRD created")
	} else if apierrors.IsAlreadyExists(err) {
		log.Println("Database CRD already exsists")
	} else {
		panic(err)
	}

	_, controller := cache.NewInformer(
		crdclient.NewListWatch(),
		&crd.Database{},
		time.Minute*10,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				createDatabase(*db)
			},
			DeleteFunc: func(obj interface{}) {
				db := obj.(*crd.Database)
				fmt.Printf("deleting database: %s \n", db.Name)
				deleteDatabase(*db)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				//fmt.Printf("Update old: %s \n      New: %s\n", oldObj, newObj)
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)

	// Wait forever
	select {}
}
