package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tamalsaha/rancid-syncer/api/management/v1alpha1"
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
	appcatalog "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/apis/shared"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
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
	cfg, rmc, kc, err := NewClient()
	if err != nil {
		return err
	}

	pcfg, err := SetupClusterForPrometheus(cfg, kc, rmc, types.NamespacedName{
		Namespace: "cattle-monitoring-system",
		Name:      "rancher-monitoring-prometheus",
	})
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(pcfg)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	os.Exit(1)

	svc, err := FindServiceForPrometheus(rmc, types.NamespacedName{
		Namespace: "cattle-monitoring-system",
		Name:      "rancher-monitoring-prometheus",
	})
	if err != nil {
		return err
	}
	fmt.Println(svc.Name)

	rancher, err := IsRancherManaged(kc)
	if err != nil {
		return err
	}
	fmt.Println("IsRancherManaged", rancher)

	projects, err := ListRancherProjects(kc)
	if err != nil {
		return err
	}
	data, err = yaml.Marshal(projects)
	if err != nil {
		return err
	}
	fmt.Println(string(data))

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

const labelKeyRancherProjectId = "field.cattle.io/projectId"

func ListRancherProjects(kc client.Client) ([]v1alpha1.Project, error) {
	var list core.NamespaceList
	err := kc.List(context.TODO(), &list)
	if meta.IsNoMatchError(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	projects := map[string]v1alpha1.Project{}
	for _, ns := range list.Items {
		projectId, exists := ns.Labels[labelKeyRancherProjectId]
		if !exists {
			continue
		}

		project, exists := projects[projectId]
		if !exists {
			project = v1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectId,
				},
				Spec: v1alpha1.ProjectSpec{
					Type:       v1alpha1.ProjectUser,
					Namespaces: nil,
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelKeyRancherProjectId: projectId,
						},
					},
					// Quota: core.ResourceRequirements{},
				},
			}
		}
		if ns.Name == metav1.NamespaceDefault {
			project.Spec.Type = v1alpha1.ProjectDefault
		} else if ns.Name == metav1.NamespaceSystem {
			project.Spec.Type = v1alpha1.ProjectSystem
		}
		project.Spec.Namespaces = append(project.Spec.Namespaces, ns.Name)

		projects[projectId] = project
	}

	result := make([]v1alpha1.Project, 0, len(projects))
	for _, p := range projects {
		result = append(result, p)
	}
	return result, nil
}

func GetRancherProject(kc client.Client, name string) (*v1alpha1.Project, error) {
	var list core.NamespaceList
	err := kc.List(context.TODO(), &list, client.MatchingLabels{
		labelKeyRancherProjectId: name,
	})
	if err != nil {
		return nil, err
	}

	project := v1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.ProjectSpec{
			Type:       v1alpha1.ProjectUser,
			Namespaces: nil,
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKeyRancherProjectId: name,
				},
			},
			// Quota: core.ResourceRequirements{},
		},
	}
	for _, ns := range list.Items {
		if ns.Name == metav1.NamespaceDefault {
			project.Spec.Type = v1alpha1.ProjectDefault
		} else if ns.Name == metav1.NamespaceSystem {
			project.Spec.Type = v1alpha1.ProjectSystem
		}
		project.Spec.Namespaces = append(project.Spec.Namespaces, ns.Name)
	}

	return &project, nil
}

/*
apiVersion: meta.k8s.appscode.com/v1alpha1
kind: ResourceQuery
request:
  outputFormat: Ref
  source:
    name: rancher-monitoring-prometheus
    namespace: cattle-monitoring-system
    resource:
      group: monitoring.coreos.com
      kind: Prometheus
      version: v1
  target:
    query:
      byLabel: exposed_by
      type: GraphQL
    ref:
      group: ""
      kind: Service
response:
- name: prometheus-operated
  namespace: cattle-monitoring-system
- name: rancher-monitoring-prometheus
  namespace: cattle-monitoring-system
*/

func FindServiceForPrometheus(rmc versioned.Interface, key types.NamespacedName) (*core.Service, error) {
	q := &rsapi.ResourceQuery{
		Request: &rsapi.ResourceQueryRequest{
			Source: rsapi.SourceInfo{
				Resource: kmapi.ResourceID{
					Group:   "monitoring.coreos.com",
					Version: "v1",
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

type PrometheusConfig struct {
	Default     bool        `json:"default"`
	URL         string      `json:"url"`
	Service     ServiceSpec `json:"service"`
	BasicAuth   BasicAuth   `json:"basicAuth"`
	BearerToken string      `json:"bearerToken"`
	TLS         TLSConfig   `json:"tls"`
}

type ServiceSpec struct {
	Scheme    string `json:"scheme"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Port      string `json:"port"`
	Path      string `json:"path"`
	Query     string `json:"query"`
}

type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TLSConfig struct {
	Ca                    string `json:"ca"`
	Cert                  string `json:"cert"`
	Key                   string `json:"key"`
	ServerName            string `json:"serverName"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify"`
}

const saTrickster = "trickster"

func SetupClusterForPrometheus(cfg *rest.Config, kc client.Client, rmc versioned.Interface, key types.NamespacedName) (*PrometheusConfig, error) {
	var prom unstructured.Unstructured
	prom.SetAPIVersion("monitoring.coreos.com/v1")
	prom.SetKind("Prometheus")
	err := kc.Get(context.TODO(), key, &prom)
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
			Group:   "monitoring.coreos.com",
			Version: "v1",
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
			Group:   "monitoring.coreos.com",
			Version: "v1",
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

	// http-web

	var pcfg PrometheusConfig
	pcfg.Service = ServiceSpec{
		Scheme:    "http",
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Port:      "",
		Path:      "",
		Query:     "",
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == "http-web" {
			pcfg.Service.Port = fmt.Sprintf("%d", p.Port)
		}
	}
	pcfg.URL = fmt.Sprintf("%s/api/v1/namespaces/%s/services/%s:%s:%s/proxy/", cfg.Host, pcfg.Service.Namespace, pcfg.Service.Scheme, pcfg.Service.Name, pcfg.Service.Port)
	// remove basic auth and client cert auth
	pcfg.BasicAuth = BasicAuth{}
	pcfg.TLS.Cert = ""
	pcfg.TLS.Key = ""
	pcfg.BearerToken = string(tokenData)
	pcfg.TLS.Ca = string(caData)

	return &pcfg, nil
}

func IsDefaultPrometheus(kc client.Client, gvk schema.GroupVersionKind, key types.NamespacedName) (bool, error) {
	rancher, err := IsRancherManaged(kc)
	if err != nil {
		return false, err
	}
	if rancher {
		if key.Namespace == metav1.NamespaceSystem {
			return true, nil
		}

		var ns core.Namespace
		err = kc.Get(context.TODO(), client.ObjectKey{Name: key.Namespace}, &ns)
		if err != nil {
			return false, err
		}
		projectId, exists := ns.Labels[labelKeyRancherProjectId]
		if !exists {
			return false, nil
		}

		var sysNS core.Namespace
		err = kc.Get(context.TODO(), client.ObjectKey{Name: metav1.NamespaceSystem}, &sysNS)
		if err != nil {
			return false, err
		}

		sysProjectId, exists := ns.Labels[labelKeyRancherProjectId]
		if !exists {
			return false, nil
		}
		return projectId == sysProjectId, nil
	}

	var list unstructured.UnstructuredList
	list.SetGroupVersionKind(gvk)
	err = kc.List(context.TODO(), &list, client.InNamespace(key.Namespace))
	if err != nil {
		return false, err
	}
	return len(list.Items) == 1, nil
}

func HandleDefaultPrometheus(kc client.Client, gvk schema.GroupVersionKind, key types.NamespacedName) ([]appcatalog.AppBinding, error) {

}
