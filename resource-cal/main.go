package main

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"sort"
	"strings"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/apiversion"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	resourcemetrics "kmodules.xyz/resource-metrics"
	"kmodules.xyz/resource-metrics/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
)

func NewClient() (discovery.DiscoveryInterface, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	ctrl.SetLogger(klogr.New())
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
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
	return disc, kc, err
}

func main() {
	if err := useKubebuilderClient(); err != nil {
		panic(err)
	}
}

func useKubebuilderClient() error {
	fmt.Println("Using kubebuilder client")
	disc, kc, err := NewClient()
	if err != nil {
		return err
	}

	var pj v1alpha1.ProjectQuota
	err = kc.Get(context.TODO(), client.ObjectKey{Name: "p-demo"}, &pj)
	if err != nil {
		return err
	}

	out, err := CalculateStatus(disc, kc, &pj)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	/*
		apiTypes, err := ListKinds(disc)
		if err != nil {
			return err
		}
		data, err := yaml.Marshal(apiTypes)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	*/

	return nil
}

type APIType struct {
	Group      string
	Kind       string
	Resource   string
	Versions   []string
	Namespaced bool
}

// handle non-namespaced resource limits
func ListKinds(disc discovery.DiscoveryInterface) (map[string]APIType, error) {
	_, resourceList, err := disc.ServerGroupsAndResources()

	apiTypes := map[string]APIType{}
	if discovery.IsGroupDiscoveryFailedError(err) || err == nil {
		for _, resources := range resourceList {
			gv, err := schema.ParseGroupVersion(resources.GroupVersion)
			if err != nil {
				return nil, err
			}
			for _, resource := range resources.APIResources {
				if strings.ContainsRune(resource.Name, '/') {
					continue
				}

				gk := schema.GroupKind{
					Group: gv.Group,
					Kind:  resource.Kind,
				}
				x, found := apiTypes[gk.String()]
				if !found {
					x = APIType{
						Group:      gv.Group,
						Kind:       resource.Kind,
						Resource:   resource.Name,
						Versions:   []string{gv.Version},
						Namespaced: resource.Namespaced,
					}
				} else {
					x.Versions = append(x.Versions, gv.Version)
				}
				apiTypes[gk.String()] = x
			}
		}
	}

	for gk, x := range apiTypes {
		if len(x.Versions) > 1 {
			sort.Slice(x.Versions, func(i, j int) bool {
				return apiversion.MustCompare(x.Versions[i], x.Versions[j]) > 0
			})
			apiTypes[gk] = x
		}
	}

	return apiTypes, nil
}

func CalculateStatus(disc discovery.DiscoveryInterface, kc client.Client, in *v1alpha1.ProjectQuota) (*v1alpha1.ProjectQuota, error) {
	var nsList core.NamespaceList
	err := kc.List(context.TODO(), &nsList, client.MatchingLabels{
		clustermeta.LabelKeyRancherFieldProjectId: in.Name,
	})
	if err != nil {
		return nil, err
	}

	// init status
	in.Status.Quotas = make([]v1alpha1.ResourceQuotaStatus, len(in.Spec.Quotas))
	for i := range in.Spec.Quotas {
		in.Status.Quotas[i] = v1alpha1.ResourceQuotaStatus{
			ResourceQuotaSpec: in.Spec.Quotas[i],
			Used:              core.ResourceList{},
		}
	}

	apiTypes, err := ListKinds(disc)
	if err != nil {
		return nil, err
	}

	for _, ns := range nsList.Items {
		nsUsed := map[schema.GroupKind]core.ResourceList{}

		for i, quota := range in.Status.Quotas {
			gk := schema.GroupKind{
				Group: quota.Group,
				Kind:  quota.Kind,
			}
			used, found := nsUsed[gk]
			if found {
				quota.Used = used
				in.Status.Quotas[i] = quota
			} else if quota.Kind == "" {
				for _, typeInfo := range apiTypes {
					if typeInfo.Group == quota.Group {
						used, found := nsUsed[schema.GroupKind{
							Group: typeInfo.Group,
							Kind:  typeInfo.Kind,
						}]
						if !found {
							used, err = UsedQuota(kc, ns.Name, typeInfo)
							if err != nil {
								return nil, err
							}
							nsUsed[gk] = used
						}
						quota.Used = AddResourceList(quota.Used, used)
					}
				}
			} else {
				typeInfo, found := apiTypes[gk.String()]
				if !found {
					return nil, fmt.Errorf("can't detect api type info for %+v", gk)
				}
				used, err := UsedQuota(kc, ns.Name, typeInfo)
				if err != nil {
					return nil, err
				}
				nsUsed[gk] = used
				quota.Used = AddResourceList(quota.Used, used)
			}

			in.Status.Quotas[i] = quota
		}
	}

	return in, nil
}

func UsedQuota(kc client.Client, ns string, typeInfo APIType) (core.ResourceList, error) {
	gk := schema.GroupKind{
		Group: typeInfo.Group,
		Kind:  typeInfo.Kind,
	}

	if !typeInfo.Namespaced {
		// Todo:
		// No opinion?
		return nil, fmt.Errorf("can't apply quota for non-namespaced resources %+v", gk)
	}

	var done bool
	var used core.ResourceList
	for _, version := range typeInfo.Versions {
		gvk := gk.WithVersion(version)
		if api.IsRegistered(gvk) {
			done = true

			var list unstructured.UnstructuredList
			list.SetGroupVersionKind(gvk)
			err := kc.List(context.TODO(), &list, client.InNamespace(ns))
			if err != nil {
				return nil, err
			}

			for _, obj := range list.Items {
				content := obj.UnstructuredContent()

				usage := core.ResourceList{}

				// https://kubernetes.io/docs/concepts/policy/resource-quotas/#compute-resource-quota
				requests, err := resourcemetrics.AppResourceRequests(content)
				if err != nil {
					return nil, err
				}
				for k, v := range requests {
					usage["requests."+k] = v
				}
				limits, err := resourcemetrics.AppResourceLimits(content)
				if err != nil {
					return nil, err
				}
				for k, v := range limits {
					usage["limits."+k] = v
				}

				used = AddResourceList(used, usage)
			}
			break
		}
	}
	if !done {
		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(gk.WithVersion(typeInfo.Versions[0]))
		err := kc.List(context.TODO(), &list, client.InNamespace(ns))
		if err != nil {
			return nil, err
		}
		if len(list.Items) > 0 {
			// Todo:
			// Don't error out
			return nil, fmt.Errorf("resource calculator not defined for %+v", gk)
		}
	}
	return used, nil
}

func AddResourceList(x, y core.ResourceList) core.ResourceList {
	names := sets.NewString()
	for k := range x {
		names.Insert(string(k))
	}
	for k := range y {
		names.Insert(string(k))
	}

	result := core.ResourceList{}
	for _, fullName := range names.UnsortedList() {
		_, name, found := strings.Cut(fullName, ".")
		var rf resource.Format
		if found {
			rf = resourceFormat(core.ResourceName(name))
		} else {
			rf = resourceFormat(core.ResourceName(fullName))
		}

		sum := resource.Quantity{Format: rf}
		sum.Add(*x.Name(core.ResourceName(fullName), rf))
		sum.Add(*y.Name(core.ResourceName(fullName), rf))
		if !sum.IsZero() {
			result[core.ResourceName(fullName)] = sum
		}
	}
	return result
}

func resourceFormat(name core.ResourceName) resource.Format {
	switch name {
	case core.ResourceCPU:
		return resource.DecimalSI
	case core.ResourceMemory:
		return resource.BinarySI
	case core.ResourceStorage:
		return resource.BinarySI
	case core.ResourcePods:
		return resource.DecimalSI
	case core.ResourceEphemeralStorage:
		return resource.BinarySI
	}
	return resource.BinarySI // panic ?
}
