package main

import (
	"context"
	"fmt"
	"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	cu "kmodules.xyz/client-go/client"
	clustermeta "kmodules.xyz/client-go/cluster"
	meta_util "kmodules.xyz/client-go/meta"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sort"
	"strings"
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
			Namespace: "monitoring",
			Name:      "panopticon",
			//Namespace: "kubeops",
			//Name:      "kube-ui-server",
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

	// services
	srcServices := map[client.ObjectKey]core.Service{}
	svcSel, err := metav1.LabelSelectorAsSelector(&svcMon.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, ns := range svcMon.Spec.NamespaceSelector.MatchNames {
		var svcList core.ServiceList
		err = kc.List(context.TODO(), &svcList, client.InNamespace(ns), client.MatchingLabelsSelector{
			Selector: svcSel,
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, svc := range svcList.Items {
			srcServices[client.ObjectKeyFromObject(&svc)] = svc
		}
	}

	// secret
	var srcSecrets []core.Secret
	for i := range svcMon.Spec.Endpoints {
		e := svcMon.Spec.Endpoints[i]

		if e.TLSConfig != nil &&
			e.TLSConfig.CA.Secret != nil &&
			e.TLSConfig.CA.Secret.Name != "" {

			key := client.ObjectKey{
				Name:      e.TLSConfig.CA.Secret.Name,
				Namespace: svcMon.Namespace,
			}

			var srcSecret core.Secret
			err := kc.Get(context.TODO(), key, &srcSecret)
			if err != nil {
				return ctrl.Result{}, err
			}
			srcSecrets = append(srcSecrets, srcSecret)

			// copySecret(kc, e.TLSConfig.CA.Secret.Name, prom.Namespace)
		}
	}

	var promList monitoringv1.PrometheusList
	if err := kc.List(context.TODO(), &promList); err != nil {
		log.Error(err, "unable to list Prometheus")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var errList []error
	for _, prom := range promList.Items {
		isDefault := IsDefaultPrometheus(prom)

		if !isDefault && prom.Namespace == req.Namespace {
			err := fmt.Errorf("federated service monitor can't be in the same namespace with project Prometheus %s/%s", prom.Namespace, prom.Namespace)
			log.Error(err, "bad service monitor")
			return ctrl.Result{}, nil // don't retry until svcmon changes
		}

		if isDefault {
			if err := updateServiceMonitorLabels(kc, prom, &svcMon); err != nil {
				errList = append(errList, err)
			}
		} else {
			targetSvcMon, err := copyServiceMonitor(kc, prom, &svcMon)
			if err != nil {
				errList = append(errList, err)
			}

			for _, srcSvc := range srcServices {
				if err := copyService(kc, &srcSvc, targetSvcMon); err != nil {
					errList = append(errList, err)
				}
				var srcEP core.Endpoints
				err = kc.Get(context.TODO(), client.ObjectKeyFromObject(&srcSvc), &srcEP)
				if err != nil {
					errList = append(errList, err)
				} else {
					if err := copyEndpoints(kc, &srcSvc, &srcEP, targetSvcMon); err != nil {
						errList = append(errList, err)
					}
				}
			}

			for _, srcSecret := range srcSecrets {
				if err := copySecret(kc, &srcSecret, targetSvcMon); err != nil {
					errList = append(errList, err)
				}
			}
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

func updateServiceMonitorLabels(kc client.Client, prom *monitoringv1.Prometheus, src *monitoringv1.ServiceMonitor) error {
	vt, err := cu.CreateOrPatch(context.TODO(), kc, src, func(in client.Object, createOp bool) client.Object {
		obj := in.(*monitoringv1.ServiceMonitor)

		labels, _ := meta_util.LabelsForLabelSelector(prom.Spec.ServiceMonitorSelector)
		obj.Labels = meta_util.OverwriteKeys(obj.Labels, labels)

		return obj
	})
	if err != nil {
		return err
	}
	klog.Infof("%s ServiceMonitor %s/%s", vt, src.Namespace, src.Name)
	return nil
}

func copyServiceMonitor(kc client.Client, prom *monitoringv1.Prometheus, src *monitoringv1.ServiceMonitor) (*monitoringv1.ServiceMonitor, error) {
	sel, err := metav1.LabelSelectorAsSelector(prom.Spec.ServiceMonitorNamespaceSelector)
	if err != nil {
		return nil, err
	}

	var nsList core.NamespaceList
	err = kc.List(context.TODO(), &nsList, client.MatchingLabelsSelector{
		Selector: sel,
	})
	if err != nil {
		return nil, err
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		if ns.Name == fmt.Sprintf("cattle-project-%s", ns.Labels[clustermeta.LabelKeyRancherFieldProjectId]) {
			continue
		}
		namespaces = append(namespaces, ns.Name)
	}
	sort.Strings(namespaces)

	target := monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      src.Name,
			Namespace: prom.Namespace,
		},
	}
	vt, err := cu.CreateOrPatch(context.TODO(), kc, &target, func(in client.Object, createOp bool) client.Object {
		obj := in.(*monitoringv1.ServiceMonitor)

		ref := metav1.NewControllerRef(prom, schema.GroupVersionKind{
			Group:   monitoring.GroupName,
			Version: monitoringv1.Version,
			Kind:    "Prometheus",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}

		labels, _ := meta_util.LabelsForLabelSelector(prom.Spec.ServiceMonitorSelector)
		obj.Labels = meta_util.OverwriteKeys(obj.Labels, labels)

		obj.Spec = *src.Spec.DeepCopy()

		keepNSMetrics := monitoringv1.RelabelConfig{
			Action:       "keep",
			SourceLabels: []monitoringv1.LabelName{"namespace"},
			Regex:        "(" + strings.Join(namespaces, "|") + ")",
		}
		for i := range obj.Spec.Endpoints {
			e := obj.Spec.Endpoints[i]

			e.HonorLabels = true // keep original labels

			if len(e.RelabelConfigs) == 0 || !reflect.DeepEqual(keepNSMetrics, *e.RelabelConfigs[0]) {
				e.RelabelConfigs = append([]*monitoringv1.RelabelConfig{
					&keepNSMetrics,
				}, e.RelabelConfigs...)
			}
			obj.Spec.Endpoints[i] = e
		}

		return obj
	})
	if err != nil {
		return nil, err
	}
	klog.Infof("%s ServiceMonitor %s/%s", vt, target.Namespace, target.Name)
	return &target, nil
}

func copySecret(kc client.Client, src *core.Secret, targetSvcMon *monitoringv1.ServiceMonitor) error {
	target := core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      src.Name,
			Namespace: targetSvcMon.Namespace,
		},
		Immutable:  nil,
		Data:       nil,
		StringData: nil,
		Type:       "",
	}

	vt, err := cu.CreateOrPatch(context.TODO(), kc, &target, func(in client.Object, createOp bool) client.Object {
		obj := in.(*core.Secret)

		ref := metav1.NewControllerRef(targetSvcMon, schema.GroupVersionKind{
			Group:   monitoring.GroupName,
			Version: monitoringv1.Version,
			Kind:    "ServiceMonitor",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}

		obj.Immutable = src.Immutable
		obj.Data = src.Data
		obj.Type = src.Type

		return obj
	})
	if err != nil {
		return err
	}
	klog.Infof("%s Secret %s/%s", vt, target.Namespace, target.Name)
	return nil
}

func copyService(kc client.Client, src *core.Service, targetSvcMon *monitoringv1.ServiceMonitor) error {
	target := core.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      src.Name,
			Namespace: targetSvcMon.Namespace,
		},
	}

	vt, err := cu.CreateOrPatch(context.TODO(), kc, &target, func(in client.Object, createOp bool) client.Object {
		obj := in.(*core.Service)

		ref := metav1.NewControllerRef(targetSvcMon, schema.GroupVersionKind{
			Group:   monitoring.GroupName,
			Version: monitoringv1.Version,
			Kind:    "ServiceMonitor",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}
		obj.Labels = meta_util.OverwriteKeys(obj.Labels, src.Labels)
		obj.Annotations = meta_util.OverwriteKeys(obj.Annotations, src.Annotations)

		obj.Spec.Type = core.ServiceTypeClusterIP

		for _, port := range src.Spec.Ports {
			obj.Spec.Ports = UpsertServicePort(obj.Spec.Ports, port)
		}
		return obj
	})
	if err != nil {
		return err
	}
	klog.Infof("%s Service %s/%s", vt, target.Namespace, target.Name)
	return nil
}

func UpsertServicePort(ports []core.ServicePort, x core.ServicePort) []core.ServicePort {
	for i, port := range ports {
		if port.Name == x.Name {
			port.Port = x.Port
			port.TargetPort = intstr.FromInt(int(x.Port))
			ports[i] = port
			return ports
		}
	}
	return append(ports, core.ServicePort{
		Name:       x.Name,
		Port:       x.Port,
		TargetPort: intstr.FromInt(int(x.Port)),
	})
}

func copyEndpoints(kc client.Client, srcSvc *core.Service, srcEP *core.Endpoints, targetSvcMon *monitoringv1.ServiceMonitor) error {
	target := core.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      srcEP.Name,
			Namespace: targetSvcMon.Namespace,
		},
	}

	vt, err := cu.CreateOrPatch(context.TODO(), kc, &target, func(in client.Object, createOp bool) client.Object {
		obj := in.(*core.Endpoints)

		ref := metav1.NewControllerRef(targetSvcMon, schema.GroupVersionKind{
			Group:   monitoring.GroupName,
			Version: monitoringv1.Version,
			Kind:    "ServiceMonitor",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}
		obj.Labels = meta_util.OverwriteKeys(obj.Labels, srcEP.Labels)
		obj.Annotations = meta_util.OverwriteKeys(obj.Annotations, srcEP.Annotations)

		for i, srcSubNet := range srcEP.Subsets {
			if i >= len(obj.Subsets) {
				obj.Subsets = append(obj.Subsets, core.EndpointSubset{})
			}

			obj.Subsets[i].Addresses = []core.EndpointAddress{
				{
					IP: srcSvc.Spec.ClusterIP,
				},
			}
			for _, port := range srcSubNet.Ports {
				obj.Subsets[i].Ports = UpsertEndpointPort(obj.Subsets[i].Ports, core.EndpointPort{
					Name: port.Name,
					Port: ServicePortForName(srcSvc, port.Name),
				})
			}
		}

		return obj
	})
	if err != nil {
		return err
	}
	klog.Infof("%s Endpoints %s/%s", vt, target.Namespace, target.Name)
	return nil
}

func ServicePortForName(svc *core.Service, portName string) int32 {
	for _, port := range svc.Spec.Ports {
		if port.Name == portName {
			return port.Port
		}
	}
	panic(fmt.Errorf("service %s/%s has no port with name %s", svc.Namespace, svc.Name, portName))
}

func UpsertEndpointPort(ports []core.EndpointPort, x core.EndpointPort) []core.EndpointPort {
	for i, port := range ports {
		if port.Name == x.Name {
			port.Port = x.Port
			ports[i] = port
			return ports
		}
	}
	return append(ports, core.EndpointPort{
		Name: x.Name,
		Port: x.Port,
	})
}
