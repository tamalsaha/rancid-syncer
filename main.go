package main

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	kubedbscheme "kubedb.dev/apimachinery/client/clientset/versioned/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func NewClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	// NOTE: Register KubeDB api types
	_ = kubedbscheme.AddToScheme(scheme)

	ctrl.SetLogger(klogr.New())
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{
		Scheme: scheme,
		Mapper: mapper,
		//Opts: client.WarningHandlerOptions{
		//	SuppressWarnings:   false,
		//	AllowDuplicateLogs: false,
		//},
	})
}

func main() {
	if err := useKubebuilderClient(); err != nil {
		panic(err)
	}
}

func useKubebuilderClient() error {
	fmt.Println("Using kubebuilder client")
	kc, err := NewClient()
	if err != nil {
		return err
	}

	rancher, err := IsRancherManaged(kc)
	if err != nil {
		return err
	}
	fmt.Println("IsRancherManaged", rancher)
	return nil
}

/*
> k get clusters.management.cattle.io local -o yaml

apiVersion: management.cattle.io/v3
kind: Cluster
metadata:
  annotations:
    provisioner.cattle.io/encrypt-migrated: "true"
  creationTimestamp: "2023-09-17T19:58:59Z"
  generation: 2
  name: local
  resourceVersion: "1994"
  uid: 920a0c9a-7f8a-46c7-8ad2-b97000e1a073
*/

func IsRancherManaged(kc client.Client) (bool, error) {
	var obj unstructured.Unstructured
	obj.SetAPIVersion("management.cattle.io/v3")
	obj.SetKind("Cluster")

	key := client.ObjectKey{
		Name: "local",
	}
	err := kc.Get(context.TODO(), key, &obj)
	if err == nil {
		return true, nil
	} else if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}
