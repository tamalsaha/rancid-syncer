package main

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	_ = monitoringv1.AddToScheme(scheme)

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

	_, err = Reconcile(context.TODO(), kc, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "",
			Name:      "",
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func Reconcile(ctx context.Context, kc client.Client, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	return ctrl.Result{}, nil
}
