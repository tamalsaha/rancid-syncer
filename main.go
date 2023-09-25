package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	openvizapi "go.openviz.dev/apimachinery/apis/openviz/v1alpha1"
	"gomodules.xyz/pointer"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	kutil "kmodules.xyz/client-go"
	kmapi "kmodules.xyz/client-go/api/v1"
	cu "kmodules.xyz/client-go/client"
	clustermanger "kmodules.xyz/client-go/cluster"
	clustermeta "kmodules.xyz/client-go/cluster"
	meta_util "kmodules.xyz/client-go/meta"
	appcatalog "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	rscoreapi "kmodules.xyz/resource-metadata/apis/core/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/apis/shared"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
	chartsapi "x-helm.dev/apimachinery/apis/charts/v1alpha1"
)

func NewClient() (*rest.Config, versioned.Interface, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = monitoringv1.AddToScheme(scheme)
	_ = chartsapi.AddToScheme(scheme)

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
	cfg, rmc, kc, err := NewClient()
	if err != nil {
		return err
	}

	projects, err := ListRancherProjects(kc)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(projects)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	os.Exit(1)

	var prom monitoringv1.Prometheus
	err = kc.Get(context.TODO(), types.NamespacedName{
		//Name:      "cattle-project-p-mrbgq-mon-prometheus",
		//Namespace: "cattle-project-p-mrbgq-monitoring",

		Namespace: "cattle-monitoring-system",
		Name:      "rancher-monitoring-prometheus",
	}, &prom)
	if err != nil {
		return err
	}
	ns, err := NamespaceForPreset(kc, &prom)
	if err != nil {
		return err
	}
	fmt.Println(ns)
	os.Exit(1)

	pcfg, err := SetupClusterForPrometheus(cfg, kc, rmc, types.NamespacedName{
		Namespace: "cattle-monitoring-system",
		Name:      "rancher-monitoring-prometheus",
	})
	if err != nil {
		return err
	}
	data, err = yaml.Marshal(pcfg)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	// os.Exit(1)
	// -----------------------------------------------------------

	svc, err := FindServiceForPrometheus(rmc, types.NamespacedName{
		Namespace: "cattle-monitoring-system",
		Name:      "rancher-monitoring-prometheus",
	})
	if err != nil {
		return err
	}
	fmt.Println(svc.Name)

	rancher := clustermanger.IsRancherManaged(kc.RESTMapper())
	fmt.Println("IsRancherManaged", rancher)

	return nil
}

func FindServiceForPrometheus(rmc versioned.Interface, key types.NamespacedName) (*core.Service, error) {
	q := &rsapi.ResourceQuery{
		Request: &rsapi.ResourceQueryRequest{
			Source: rsapi.SourceInfo{
				Resource: kmapi.ResourceID{
					Group:   monitoring.GroupName,
					Version: monitoringv1.Version,
					Kind:    "Prometheus",
				},
				Namespace: key.Namespace,
				Name:      key.Name,
			},
			Target: &shared.ResourceLocator{
				Ref: metav1.GroupKind{
					Group: "",
					Kind:  "Service",
				},
				Query: shared.ResourceQuery{
					Type:    shared.GraphQLQuery,
					ByLabel: kmapi.EdgeLabelExposedBy,
				},
			},
			OutputFormat: rsapi.OutputFormatObject,
		},
		Response: nil,
	}
	var err error
	q, err = rmc.MetaV1alpha1().ResourceQueries().Create(context.TODO(), q, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	var list core.ServiceList
	err = json.Unmarshal(q.Response.Raw, &list)
	if err != nil {
		return nil, err
	}
	for _, svc := range list.Items {
		if svc.Spec.ClusterIP != "None" {
			return &svc, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    "",
		Resource: "services",
	}, key.String())
}

const (
	portPrometheus = "http-web"
	saTrickster    = "trickster"
)

func SetupClusterForPrometheus(cfg *rest.Config, kc client.Client, rmc versioned.Interface, key types.NamespacedName) (*mona.PrometheusConfig, error) {
	cm := clustermanger.DetectClusterManager(kc)

	gvk := schema.GroupVersionKind{
		Group:   monitoring.GroupName,
		Version: monitoringv1.Version,
		Kind:    "Prometheus",
	}

	var prom monitoringv1.Prometheus
	err := kc.Get(context.TODO(), key, &prom)
	if err != nil {
		return nil, err
	}

	key = client.ObjectKeyFromObject(&prom)
	isDefault, err := clustermanger.IsDefault(kc, cm, gvk, key)
	if err != nil {
		return nil, err
	}

	svc, err := FindServiceForPrometheus(rmc, key)
	if err != nil {
		return nil, err
	}

	// https://github.com/bytebuilders/installer/blob/master/charts/monitoring-config/templates/trickster/trickster.yaml
	sa := core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saTrickster,
			Namespace: key.Namespace,
		},
	}
	savt, err := cu.CreateOrPatch(context.TODO(), kc, &sa, func(in client.Object, createOp bool) client.Object {
		obj := in.(*core.ServiceAccount)
		ref := metav1.NewControllerRef(&prom, schema.GroupVersionKind{
			Group:   monitoring.GroupName,
			Version: monitoringv1.Version,
			Kind:    "Prometheus",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}

		return obj
	})
	if err != nil {
		return nil, err
	}
	klog.Infof("%s service account %s/%s", savt, sa.Namespace, sa.Name)

	role := rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saTrickster,
			Namespace: key.Namespace,
		},
	}
	rolevt, err := cu.CreateOrPatch(context.TODO(), kc, &role, func(in client.Object, createOp bool) client.Object {
		obj := in.(*rbac.Role)
		ref := metav1.NewControllerRef(&prom, schema.GroupVersionKind{
			Group:   monitoring.GroupName,
			Version: monitoringv1.Version,
			Kind:    "Prometheus",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}

		obj.Rules = []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services/proxy"},
				Verbs:     []string{"*"},
			},
		}

		return obj
	})
	if err != nil {
		return nil, err
	}
	klog.Infof("%s role %s/%s", rolevt, role.Namespace, role.Name)

	rb := rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saTrickster,
			Namespace: key.Namespace,
		},
	}
	rbvt, err := cu.CreateOrPatch(context.TODO(), kc, &rb, func(in client.Object, createOp bool) client.Object {
		obj := in.(*rbac.RoleBinding)
		ref := metav1.NewControllerRef(&prom, schema.GroupVersionKind{
			Group:   rbac.GroupName,
			Version: "v1",
			Kind:    "Role",
		})
		obj.OwnerReferences = []metav1.OwnerReference{*ref}

		obj.RoleRef = rbac.RoleRef{
			APIGroup: rbac.GroupName,
			Kind:     "Role",
			Name:     role.Name,
		}

		obj.Subjects = []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		}

		return obj
	})
	if err != nil {
		return nil, err
	}
	klog.Infof("%s role binding %s/%s", rbvt, rb.Namespace, rb.Name)

	err = CreatePreset(kc, cm, &prom, isDefault)
	if err != nil {
		return nil, err
	}

	if isDefault {
		// create Prometheus AppBinding
		vt, err := CreatePrometheusAppBinding(kc, &prom, svc)
		if err != nil {
			return nil, err
		}
		if vt == kutil.VerbCreated {
			RegisterPrometheus()
		}

		// create Grafana AppBinding
	}

	var caData, tokenData []byte
	err = wait.PollImmediate(kutil.RetryInterval, kutil.ReadinessTimeout, func() (done bool, err error) {
		var sacc core.ServiceAccount
		err = kc.Get(context.TODO(), client.ObjectKeyFromObject(&sa), &sacc)
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		if len(sacc.Secrets) == 0 {
			return false, nil
		}

		skey := client.ObjectKey{
			Namespace: sa.Namespace,
			Name:      sacc.Secrets[0].Name,
		}
		var s core.Secret
		err = kc.Get(context.TODO(), skey, &s)
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}

		var caFound, tokenFound bool
		caData, caFound = s.Data["ca.crt"]
		tokenData, tokenFound = s.Data["token"]
		return caFound && tokenFound, nil
	})
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	var pcfg mona.PrometheusConfig
	pcfg.Service = mona.ServiceSpec{
		Scheme:    "http",
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Port:      "",
		Path:      "",
		Query:     "",
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == portPrometheus {
			pcfg.Service.Port = fmt.Sprintf("%d", p.Port)
		}
	}
	pcfg.URL = fmt.Sprintf("%s/api/v1/namespaces/%s/services/%s:%s:%s/proxy/", cfg.Host, pcfg.Service.Namespace, pcfg.Service.Scheme, pcfg.Service.Name, pcfg.Service.Port)
	// remove basic auth and client cert auth
	pcfg.BasicAuth = mona.BasicAuth{}
	pcfg.TLS.Cert = ""
	pcfg.TLS.Key = ""
	pcfg.BearerToken = string(tokenData)
	pcfg.TLS.Ca = string(caData)

	return &pcfg, nil
}

const presetsMonitoring = "monitoring-presets"

var defaultPresetsLabels = map[string]string{
	"charts.x-helm.dev/is-default-preset": "true",
}

func CreatePreset(kc client.Client, cm kmapi.ClusterManager, p *monitoringv1.Prometheus, isDefault bool) error {
	presets := GeneratePresetForPrometheus(*p)
	presetBytes, err := json.Marshal(presets)
	if err != nil {
		return err
	}

	if cm.ManagedByRancher() {

		if isDefault {
			// create ClusterChartPreset
			err := CreateClusterPreset(kc, presetBytes)
			if err != nil {
				return err
			}
		} else {
			// create ChartPreset
			err2 := CreateProjectPreset(kc, p, presetBytes)
			if err2 != nil {
				return err2
			}
		}
		return nil
	} // create ClusterChartPreset
	err = CreateClusterPreset(kc, presetBytes)
	return err
}

func CreateClusterPreset(kc client.Client, presetBytes []byte) error {
	ccp := chartsapi.ClusterChartPreset{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: presetsMonitoring,
		},
	}
	vt, err := cu.CreateOrPatch(context.TODO(), kc, &ccp, func(in client.Object, createOp bool) client.Object {
		obj := in.(*chartsapi.ClusterChartPreset)

		obj.Labels = defaultPresetsLabels
		obj.Spec = chartsapi.ClusterChartPresetSpec{
			Values: &runtime.RawExtension{
				Raw: presetBytes,
			},
		}

		return obj
	})
	if err != nil {
		return err
	}
	klog.Infof("%s ClusterChartPreset %s", vt, ccp.Name)
	return nil
}

func CreateProjectPreset(kc client.Client, p *monitoringv1.Prometheus, presetBytes []byte) error {
	cp := chartsapi.ChartPreset{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      presetsMonitoring,
			Namespace: p.Namespace,
		},
	}
	vt, err := cu.CreateOrPatch(context.TODO(), kc, &cp, func(in client.Object, createOp bool) client.Object {
		obj := in.(*chartsapi.ChartPreset)

		obj.Labels = defaultPresetsLabels
		obj.Spec = chartsapi.ClusterChartPresetSpec{
			Values: &runtime.RawExtension{
				Raw: presetBytes,
			},
		}

		return obj
	})
	if err != nil {
		return err
	}
	klog.Infof("%s ChartPreset %s/%s", vt, cp.Namespace, cp.Name)
	return nil
}

func GeneratePresetForPrometheus(p monitoringv1.Prometheus) mona.MonitoringPresets {
	var preset mona.MonitoringPresets

	preset.Spec.Monitoring.Agent = string(mona.AgentPrometheusOperator)
	svcmonLabels, ok := meta_util.LabelsForLabelSelector(p.Spec.ServiceMonitorSelector)
	if !ok {
		klog.Warningln("Prometheus %s/%s uses match expressions in ServiceMonitorSelector", p.Namespace, p.Name)
	}
	preset.Spec.Monitoring.ServiceMonitor.Labels = svcmonLabels

	preset.Form.Alert.Enabled = mona.SeverityFlagCritical
	ruleLabels, ok := meta_util.LabelsForLabelSelector(p.Spec.RuleSelector)
	if !ok {
		klog.Warningln("Prometheus %s/%s uses match expressions in RuleSelector", p.Namespace, p.Name)
	}
	preset.Form.Alert.Labels = ruleLabels

	return preset
}

func CreatePrometheusAppBinding(kc client.Client, p *monitoringv1.Prometheus, svc *core.Service) (kutil.VerbType, error) {
	ab := appcatalog.AppBinding{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-prometheus",
			Namespace: p.Namespace,
		},
	}

	vt, err := cu.CreateOrPatch(context.TODO(), kc, &ab, func(in client.Object, createOp bool) client.Object {
		obj := in.(*appcatalog.AppBinding)

		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations["monitoring.appscode.com/is-default-prometheus"] = "true"

		obj.Spec.Type = "Prometheus"
		obj.Spec.AppRef = &kmapi.TypedObjectReference{
			APIGroup:  monitoring.GroupName,
			Kind:      "Prometheus",
			Namespace: p.Namespace,
			Name:      p.Name,
		}
		obj.Spec.ClientConfig = appcatalog.ClientConfig{
			// URL:                   nil,
			Service: &appcatalog.ServiceReference{
				Scheme:    "http",
				Namespace: svc.Namespace,
				Name:      svc.Name,
				Port:      0,
				Path:      "",
				Query:     "",
			},
			//InsecureSkipTLSVerify: false,
			//CABundle:              nil,
			//ServerName:            "",
		}
		for _, p := range svc.Spec.Ports {
			if p.Name == portPrometheus {
				obj.Spec.ClientConfig.Service.Port = p.Port
			}
		}

		return obj
	})
	if err == nil {
		klog.Infof("%s AppBinding %s/%s", vt, ab.Namespace, ab.Name)
	}
	return vt, err
}

func RegisterPrometheus() error {
	return nil
}

func CreateGrafanaAppBinding(kc client.Client, key types.NamespacedName, config mona.GrafanaConfig) (kutil.VerbType, error) {
	ab := appcatalog.AppBinding{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-grafana",
			Namespace: key.Namespace,
		},
	}

	abvt, err := cu.CreateOrPatch(context.TODO(), kc, &ab, func(in client.Object, createOp bool) client.Object {
		obj := in.(*appcatalog.AppBinding)

		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}
		obj.Annotations["monitoring.appscode.com/is-default-grafana"] = "true"

		obj.Spec.Type = "Grafana"
		obj.Spec.AppRef = nil
		obj.Spec.ClientConfig = appcatalog.ClientConfig{
			URL: pointer.StringP(config.URL),
			//Service: &appcatalog.ServiceReference{
			//	Scheme:    "http",
			//	Namespace: svc.Namespace,
			//	Name:      svc.Name,
			//	Port:      0,
			//	Path:      "",
			//	Query:     "",
			//},
			//InsecureSkipTLSVerify: false,
			//CABundle:              nil,
			//ServerName:            "",
		}
		obj.Spec.Secret = &core.LocalObjectReference{
			Name: ab.Name + "-auth",
		}

		params := openvizapi.GrafanaConfiguration{
			TypeMeta:   metav1.TypeMeta{},
			Datasource: "",
			FolderID:   nil,
		}
		paramBytes, err := json.Marshal(params)
		if err != nil {
			panic(err)
		}
		obj.Spec.Parameters = &runtime.RawExtension{
			Raw: paramBytes,
		}

		return obj
	})
	if err == nil {
		klog.Infof("%s AppBinding %s/%s", abvt, ab.Namespace, ab.Name)

		authSecret := core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ab.Name + "-auth",
				Namespace: key.Namespace,
			},
		}

		svt, e2 := cu.CreateOrPatch(context.TODO(), kc, &authSecret, func(in client.Object, createOp bool) client.Object {
			obj := in.(*core.Secret)

			ref := metav1.NewControllerRef(&ab, schema.GroupVersionKind{
				Group:   appcatalog.SchemeGroupVersion.Group,
				Version: appcatalog.SchemeGroupVersion.Version,
				Kind:    "AppBinding",
			})
			obj.OwnerReferences = []metav1.OwnerReference{*ref}

			obj.StringData = map[string]string{
				"token": config.BearerToken,
			}

			return obj
		})
		if e2 == nil {
			klog.Infof("%s Grafana auth secret %s/%s", svt, authSecret.Namespace, authSecret.Name)
		}
	}

	return abvt, err
}

func NamespaceForPreset(kc client.Client, prom *monitoringv1.Prometheus) (string, error) {
	ls := prom.Spec.ServiceMonitorNamespaceSelector
	if ls.MatchLabels == nil {
		ls.MatchLabels = make(map[string]string)
	}
	ls.MatchLabels[clustermeta.LabelKeyRancherHelmProjectOperated] = "true"
	sel, err := metav1.LabelSelectorAsSelector(ls)
	if err != nil {
		return "", err
	}

	var nsList core.NamespaceList
	err = kc.List(context.TODO(), &nsList, client.MatchingLabelsSelector{Selector: sel})
	if err != nil {
		return "", err
	}
	namespaces := nsList.Items
	if len(namespaces) == 0 {
		return "", fmt.Errorf("failed to select AppBinding namespace for Prometheus %s/%s", prom.Namespace, prom.Name)
	}
	sort.Slice(namespaces, func(i, j int) bool {
		return namespaces[i].CreationTimestamp.Before(&namespaces[j].CreationTimestamp)
	})
	return namespaces[0].Name, nil
}

/*
apiVersion: helm.cattle.io/v1alpha1
kind: ProjectHelmChart
metadata:
  name: project-monitoring
  namespace: cattle-project-p-tkgpc

status:
  dashboardValues:
    alertmanagerURL: >-
      https://172.234.33.183/k8s/clusters/c-m-mhqtw2cs/api/v1/namespaces/cattle-project-p-tkgpc-monitoring/services/http:cattle-project-p-tkgpc-mon-alertmanager:9093/proxy
    grafanaURL: >-
      https://172.234.33.183/k8s/clusters/c-m-mhqtw2cs/api/v1/namespaces/cattle-project-p-tkgpc-monitoring/services/http:cattle-project-p-tkgpc-monitoring-grafana:80/proxy
    prometheusURL: >-
      https://172.234.33.183/k8s/clusters/c-m-mhqtw2cs/api/v1/namespaces/cattle-project-p-tkgpc-monitoring/services/http:cattle-project-p-tkgpc-mon-prometheus:9090/proxy

*/

func ListRancherProjects(kc client.Client) ([]rscoreapi.Project, error) {
	var list core.NamespaceList
	err := kc.List(context.TODO(), &list)
	if meta.IsNoMatchError(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	projects := map[string]rscoreapi.Project{}
	now := time.Now()
	for _, ns := range list.Items {
		projectId, exists := ns.Labels[clustermeta.LabelKeyRancherFieldProjectId]
		if !exists {
			continue
		}

		project, exists := projects[projectId]
		if !exists {
			project = rscoreapi.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:              projectId,
					CreationTimestamp: metav1.NewTime(now),
					UID:               types.UID(uuid.Must(uuid.NewUUID()).String()),
					Labels: map[string]string{
						clustermeta.LabelKeyRancherFieldProjectId: projectId,
					},
				},
				Spec: rscoreapi.ProjectSpec{
					Type:       rscoreapi.ProjectUser,
					Namespaces: nil,
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							clustermeta.LabelKeyRancherFieldProjectId: projectId,
						},
					},
				},
			}
		}

		if ns.CreationTimestamp.Before(&project.CreationTimestamp) {
			project.CreationTimestamp = ns.CreationTimestamp
		}

		if ns.Name == metav1.NamespaceDefault {
			project.Spec.Type = rscoreapi.ProjectDefault
		} else if ns.Name == metav1.NamespaceSystem {
			project.Spec.Type = rscoreapi.ProjectSystem
		}
		project.Spec.Namespaces = append(project.Spec.Namespaces, ns.Name)

		projects[projectId] = project
	}

	for projectId, prj := range projects {
		var hasUseNs bool
		presets := prj.Spec.Presets
		for _, ns := range prj.Spec.Namespaces {
			if !strings.HasPrefix(ns, "cattle-project-p-") {
				hasUseNs = true
			}

			if prj.Spec.Type == rscoreapi.ProjectSystem {
				if ns == metav1.NamespaceSystem {
					var ccps chartsapi.ClusterChartPresetList
					err := kc.List(context.TODO(), &ccps)
					if err != nil && !meta.IsNoMatchError(err) {
						return nil, err
					}
					for _, x := range ccps.Items {
						presets = append(presets, shared.SourceLocator{
							Resource: kmapi.ResourceID{
								Group:   chartsapi.GroupVersion.Group,
								Version: chartsapi.GroupVersion.Version,
								Kind:    chartsapi.ResourceKindClusterChartPreset,
							},
							Ref: kmapi.ObjectReference{
								Name: x.Name,
							},
						})
					}
				}
			} else {
				var cps chartsapi.ChartPresetList
				err := kc.List(context.TODO(), &cps, client.InNamespace(ns))
				if err != nil && !meta.IsNoMatchError(err) {
					return nil, err
				}
				for _, x := range cps.Items {
					presets = append(presets, shared.SourceLocator{
						Resource: kmapi.ResourceID{
							Group:   chartsapi.GroupVersion.Group,
							Version: chartsapi.GroupVersion.Version,
							Kind:    chartsapi.ResourceKindChartPreset,
						},
						Ref: kmapi.ObjectReference{
							Name:      x.Name,
							Namespace: x.Namespace,
						},
					})
				}
			}
		}

		// drop projects where all namespaces start with cattle-project-p
		if !hasUseNs {
			delete(projects, projectId)
			continue
		}

		sort.Slice(presets, func(i, j int) bool {
			if presets[i].Ref.Namespace != presets[j].Ref.Namespace {
				return presets[i].Ref.Namespace < presets[j].Ref.Namespace
			}
			return presets[i].Ref.Name < presets[j].Ref.Name
		})

		prj.Spec.Presets = presets
		projects[projectId] = prj
	}

	if clustermeta.IsRancherManaged(kc.RESTMapper()) {
		sysProjectId, _, err := clustermeta.GetSystemProjectId(kc)
		if err != nil {
			return nil, err
		}

		var promList monitoringv1.PrometheusList
		err = kc.List(context.TODO(), &promList)
		if err != nil && !meta.IsNoMatchError(err) {
			return nil, err
		}
		for _, prom := range promList.Items {
			var projectId string
			if prom.Namespace == "cattle-monitoring-system" {
				projectId = sysProjectId
			} else {
				if prom.Spec.ServiceMonitorNamespaceSelector != nil {
					projectId = prom.Spec.ServiceMonitorNamespaceSelector.MatchLabels[clustermeta.LabelKeyRancherHelmProjectId]
				}
			}

			prj, found := projects[projectId]
			if !found {
				continue
			}

			if prj.Spec.Monitoring == nil {
				prj.Spec.Monitoring = &rscoreapi.ProjectMonitoring{}
			}
			prj.Spec.Monitoring.PrometheusRef = &kmapi.ObjectReference{
				Namespace: prom.Namespace,
				Name:      prom.Name,
			}

			alertmanager, err := FindSiblingAlertManagerForPrometheus(kc, client.ObjectKeyFromObject(prom))
			if err != nil {
				return nil, err
			}
			prj.Spec.Monitoring.AlertmanagerRef = &kmapi.ObjectReference{
				Namespace: alertmanager.Namespace,
				Name:      alertmanager.Name,
			}

			if projectId == sysProjectId {
				prj.Spec.Monitoring.AlertmanagerURL = alertmanager.Spec.ExternalURL
				prj.Spec.Monitoring.PrometheusURL = prom.Spec.ExternalURL
				prj.Spec.Monitoring.GrafanaURL = strings.Replace(
					prj.Spec.Monitoring.PrometheusURL,
					"/services/http:rancher-monitoring-prometheus:9090/proxy",
					"/services/http:rancher-monitoring-grafana:80/proxy/?orgId=1",
					1)
			} else {
				prj.Spec.Monitoring.AlertmanagerURL,
					prj.Spec.Monitoring.GrafanaURL,
					prj.Spec.Monitoring.PrometheusURL = DetectProjectMonitoringURLs(kc, prom.Namespace)
			}

			projects[projectId] = prj
		}
	}

	result := make([]rscoreapi.Project, 0, len(projects))
	for _, p := range projects {
		result = append(result, p)
	}
	return result, nil
}

func FindSiblingAlertManagerForPrometheus(kc client.Client, key types.NamespacedName) (*monitoringv1.Alertmanager, error) {
	var list monitoringv1.AlertmanagerList
	err := kc.List(context.TODO(), &list, client.InNamespace(key.Namespace))
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 1 {
		klog.Warningln("multiple alert manager found in namespace %s", key.Namespace)
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	return &list.Items[0], nil
}

func DetectProjectMonitoringURLs(kc client.Client, promNS string) (alertmanagerURL, grafanaURL, prometheusURL string) {
	var prjHelm unstructured.Unstructured
	prjHelm.SetAPIVersion("helm.cattle.io/v1alpha1")
	prjHelm.SetKind("ProjectHelmChart")
	key := client.ObjectKey{
		Name:      "project-monitoring",
		Namespace: strings.TrimSuffix(promNS, "-monitoring"),
	}
	err := kc.Get(context.TODO(), key, &prjHelm)
	if err != nil {
		return
	}

	alertmanagerURL, _, _ = unstructured.NestedString(prjHelm.UnstructuredContent(), "status", "dashboardValues", "alertmanagerURL")
	grafanaURL, _, _ = unstructured.NestedString(prjHelm.UnstructuredContent(), "status", "dashboardValues", "grafanaURL")
	prometheusURL, _, _ = unstructured.NestedString(prjHelm.UnstructuredContent(), "status", "dashboardValues", "prometheusURL")
	return
}

var gr = schema.GroupResource{
	Group:    rscoreapi.SchemeGroupVersion.Group,
	Resource: rscoreapi.ResourceProjects,
}

func GetRancherProject(kc client.Client, projectId string) (*rscoreapi.Project, error) {
	projects, err := ListRancherProjects(kc)
	if err != nil {
		return nil, err
	}
	for _, prj := range projects {
		if prj.Name == projectId {
			return &prj, nil
		}
	}
	return nil, apierrors.NewNotFound(gr, projectId)
}
