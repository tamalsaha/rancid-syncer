package main

import (
	"context"
	"fmt"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/apiversion"
	clustermeta "kmodules.xyz/client-go/cluster"
	"kmodules.xyz/resource-metadata/apis/management/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
	"sort"
	"strings"
)

func NewClient() (discovery.DiscoveryInterface, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

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
	disc, _, err := NewClient()
	if err != nil {
		return err
	}

	apiTypes, err := ListKinds(disc)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(apiTypes)
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	return nil
}

type APIType struct {
	Group    string
	Kind     string
	Resource string
	Versions []string
}

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
						Group:    gv.Group,
						Kind:     resource.Kind,
						Resource: resource.Name,
						Versions: []string{gv.Version},
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

	apiTypes, err := ListKinds(disc)
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

	used := map[schema.GroupKind]core.ResourceList{}

	for _, ns := range nsList.Items {

	}

	/*
		GK
		G
		Group = "" Kind= ?

	*/

	return in, nil
}
