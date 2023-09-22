package main

import (
	"context"
	"fmt"
	"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	cu "kmodules.xyz/client-go/client"
	clustermeta "kmodules.xyz/client-go/cluster"
	meta_util "kmodules.xyz/client-go/meta"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sort"
	"strings"

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

	var promList monitoringv1.PrometheusList
	if err := kc.List(context.TODO(), &promList); err != nil {
		log.Error(err, "unable to list Prometheus")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var errList []error
	for _, prom := range promList.Items {
		if !IsDefaultPrometheus(prom) && prom.Namespace == req.Namespace {
			err := fmt.Errorf("federated service monitor can't be in the same namespace with project Prometheus %s/%s", prom.Namespace, prom.Namespace)
			log.Error(err, "bad service monitor")
			return ctrl.Result{}, nil // don't retry until svcmon changes
		}

		if err := syncServiceMonitor(kc, prom, &svcMon); err != nil {
			errList = append(errList, err)
		}
	}

	return ctrl.Result{}, errors.NewAggregate(errList)
}

func IsDefaultPrometheus(prom *monitoringv1.Prometheus) bool {
	expected := client.ObjectKey{
		Namespace: "cattle-monitoring-system",
		Name:      "rancher-monitoring-prometheus",
	}
	pk := client.ObjectKeyFromObject(prom)
	return pk == expected
}

func syncServiceMonitor(kc client.Client, prom *monitoringv1.Prometheus, src *monitoringv1.ServiceMonitor) error {
	sel, err := metav1.LabelSelectorAsSelector(prom.Spec.ServiceMonitorNamespaceSelector)
	if err != nil {
		return err
	}

	var nsList core.NamespaceList
	err = kc.List(context.TODO(), &nsList, client.MatchingLabelsSelector{
		Selector: sel,
	})
	if err != nil {
		return err
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		if ns.Name == fmt.Sprintf("cattle-project-%s", ns.Labels[clustermeta.LabelKeyRancherFieldProjectId]) {
			continue
		}
		namespaces = append(namespaces, ns.Name)
	}
	sort.Strings(namespaces)

	cp := monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      src.Name,
			Namespace: prom.Namespace,
		},
	}
	vt, err := cu.CreateOrPatch(context.TODO(), kc, &cp, func(in client.Object, createOp bool) client.Object {
		obj := in.(*monitoringv1.ServiceMonitor)

		ref := metav1.NewControllerRef(prom, schema.GroupVersionKind{
			Group:   monitoring.GroupName,
			Version: monitoringv1.Version,
			Kind:    "ServiceMonitor",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}

		labels, _ := meta_util.LabelsForLabelSelector(prom.Spec.ServiceMonitorSelector)
		obj.Labels = meta_util.OverwriteKeys(obj.Labels, labels)

		obj.Spec = src.Spec

		for _, e := range obj.Spec.Endpoints {
			e.RelabelConfigs = append([]*monitoringv1.RelabelConfig{
				{
					Action:       "keep",
					SourceLabels: []monitoringv1.LabelName{"namespace"},
					Regex:        strings.Join(namespaces, "|"),
				},
			}, e.RelabelConfigs...)
		}

		return obj
	})
	if err != nil {
		return err
	}
	klog.Infof("%s ServiceMonitor %s/%s", vt, cp.Namespace, cp.Name)
	return nil
}
