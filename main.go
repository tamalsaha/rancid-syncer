package main

import (
	"context"
	"fmt"

	"github.com/tamalsaha/rancid-syncer/api/management/v1alpha1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/apis/shared"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
)

func NewClient() (versioned.Interface, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	ctrl.SetLogger(klogr.New())
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	rmc, err := versioned.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	mapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, nil, err
	}

	kc, err := client.New(cfg, client.Options{
		Scheme: scheme,
		Mapper: mapper,
		//Opts: client.WarningHandlerOptions{
		//	SuppressWarnings:   false,
		//	AllowDuplicateLogs: false,
		//},
	})
	return rmc, kc, err
}

func main() {
	if err := useKubebuilderClient(); err != nil {
		panic(err)
	}
}

func useKubebuilderClient() error {
	fmt.Println("Using kubebuilder client")
	rmc, kc, err := NewClient()
	if err != nil {
		return err
	}

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
	data, err := yaml.Marshal(projects)
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
