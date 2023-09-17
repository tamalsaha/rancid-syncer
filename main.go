package main

import (
	"context"
	"fmt"
	"github.com/tamalsaha/rancid-syncer/api/management/v1alpha1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
)

func NewClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

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
				ObjectMeta: v1.ObjectMeta{
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
		ObjectMeta: v1.ObjectMeta{
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
