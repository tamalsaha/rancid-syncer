package main

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	clustermeta "kmodules.xyz/client-go/cluster"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
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

	var svcMon monitoringv1.ServiceMonitor
	if err := kc.Get(ctx, req.NamespacedName, &svcMon); err != nil {
		log.Error(err, "unable to fetch ServiceMonitor")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// has federate label
	val, found := svcMon.Labels[mona.FederatedKey]
	if !found || val != "true" {
		return ctrl.Result{}, nil
	}

	if !clustermeta.IsRancherManaged(kc.RESTMapper()) {
		return ctrl.Result{}, nil
	}

	if yes, err := clustermeta.IsInSystemProject(kc, req.Namespace); err != nil {
		log.Error(err, "unable to detect if in system project")
		return ctrl.Result{}, err
	} else if !yes {
		// return error?
		log.Info("can't federate service monitor that is not part of the system project")
		return ctrl.Result{}, nil
	}

	sysProjectId, err := clustermeta.GetSystemProjectId(kc)

	var promList monitoringv1.PrometheusList
	if err := kc.List(context.TODO(), &promList); err != nil {
		log.Error(err, "unable to list Prometheus")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var errList []error
	for _, prom := range promList.Items {
		siblings, err := clustermeta.AreSiblingNamespaces(kc, req.Namespace, prom.Namespace)
		if err != nil {
			errList = append(errList, err)
			continue
			// return ctrl.Result{}, err // should we do rest of the loop?
		}
		if siblings {
			continue
		}
		syncServiceMonitor(kc, prom, &svcMon)

	}

	return ctrl.Result{}, nil
}

func syncServiceMonitor(kc client.Client, prom *monitoringv1.Prometheus, src *monitoringv1.ServiceMonitor) error {

}
