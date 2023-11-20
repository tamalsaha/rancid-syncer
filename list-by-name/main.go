package main

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"time"
	chartsapi "x-helm.dev/apimachinery/apis/charts/v1alpha1"
)

func NewClient() (*rest.Config, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = chartsapi.AddToScheme(scheme)

	ctrl.SetLogger(klogr.New())
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, nil, err
	}

	kc, err := client.New(cfg, client.Options{
		Scheme: scheme,
		Mapper: mapper,
		Opts: client.WarningHandlerOptions{
			SuppressWarnings:   false,
			AllowDuplicateLogs: false,
		},
	})
	return cfg, kc, err
}

func main() {
	ctrl.SetLogger(klogr.New())
	if err := useKubebuilderClient(); err != nil {
		panic(err)
	}
	time.Sleep(1 * time.Minute)
}

func useKubebuilderClient() error {
	fmt.Println("Using kubebuilder client")
	cfg, kc, err := NewClient()
	if err != nil {
		return err
	}

	dc := dynamic.NewForConfigOrDie(cfg)
	gvr := schema.GroupVersionResource{
		Group:    "charts.x-helm.dev",
		Version:  "v1alpha1",
		Resource: "chartpresets",
	}

	fs := fields.SelectorFromSet(map[string]string{
		"metadata.name": "capi-presets",
	}).String()

	list, err := dc.Resource(gvr).List(context.TODO(), metav1.ListOptions{
		FieldSelector: fs,
	})
	if err != nil {
		return err
	}
	for _, db := range list.Items {
		fmt.Println(client.ObjectKeyFromObject(&db))
	}
	// return nil

	var pglist chartsapi.ChartPresetList
	err = kc.List(context.TODO(), &pglist, client.MatchingFields{
		"metadata.name": "capi-presets",
	})
	if err != nil {
		return err
	}
	for _, db := range pglist.Items {
		fmt.Println(client.ObjectKeyFromObject(&db))
	}
	return nil
}
