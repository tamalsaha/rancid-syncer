package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func NewClient() (*rest.Config, versioned.Interface, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	ctrl.SetLogger(klogr.New())
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	rmc, err := versioned.NewForConfig(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	kc, err := client.New(cfg, client.Options{
		Scheme: scheme,
		Mapper: mapper,
		//Opts: client.WarningHandlerOptions{
		//	SuppressWarnings:   false,
		//	AllowDuplicateLogs: false,
		//},
	})
	return cfg, rmc, kc, err
}

func main() {
	if err := useKubebuilderClient(); err != nil {
		panic(err)
	}
}

func useKubebuilderClient() error {
	fmt.Println("Using kubebuilder client")
	_, _, kc, err := NewClient()
	if err != nil {
		return err
	}

	err = ListKinds(kc)
	if err != nil {
		return err
	}

	return nil
}

func ListKinds(kc client.Client) error {
	rs, err := kc.RESTMapper().ResourcesFor(schema.GroupVersionResource{
		Group:    "kubedb.com",
		Version:  "",
		Resource: "",
	})
	if err != nil {
		return err
	}
	fmt.Println(rs)

	kinds, err := kc.RESTMapper().KindsFor(schema.GroupVersionResource{
		Group:    "kubedb.com",
		Version:  "",
		Resource: "",
	})
	if err != nil {
		return err
	}
	for _, k := range kinds {
		fmt.Println(k)
	}

	mappings, err := kc.RESTMapper().RESTMappings(schema.GroupKind{
		Group: "kubedb.com",
		Kind:  "",
	})
	if err != nil {
		return err
	}
	for _, mapping := range mappings {
		fmt.Println(mapping.GroupVersionKind)
	}

	return nil
}
