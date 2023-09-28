package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/apiversion"
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

	apiTypes, err := ListKinds(disc, "kubedb.com")
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

func ListKinds(c discovery.DiscoveryInterface, group string) (map[schema.GroupKind]APIType, error) {
	_, resourceList, err := c.ServerGroupsAndResources()

	apiTypes := map[schema.GroupKind]APIType{}
	if discovery.IsGroupDiscoveryFailedError(err) || err == nil {
		for _, resources := range resourceList {
			gv, err := schema.ParseGroupVersion(resources.GroupVersion)
			if err != nil {
				return nil, err
			}
			if gv.Group != group {
				continue
			}
			for _, resource := range resources.APIResources {
				gk := schema.GroupKind{
					Group: group,
					Kind:  resource.Kind,
				}
				x, found := apiTypes[gk]
				if !found {
					apiTypes[gk] = APIType{
						Group:    group,
						Kind:     resource.Kind,
						Resource: resource.Name,
						Versions: []string{resource.Version},
					}
				} else {
					x.Versions = append(x.Versions, resource.Version)
					apiTypes[gk] = x
				}
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

func ListAPIVersions(c discovery.DiscoveryInterface, group, kind string) ([]string, error) {
	_, resourceList, err := c.ServerGroupsAndResources()

	out := make([]string, 0)
	if discovery.IsGroupDiscoveryFailedError(err) || err == nil {
		for _, resources := range resourceList {
			gv, err := schema.ParseGroupVersion(resources.GroupVersion)
			if err != nil {
				return nil, err
			}
			if gv.Group != group {
				continue
			}
			for _, resource := range resources.APIResources {
				if resource.Kind == kind && !strings.ContainsRune(resource.Name, '/') {
					out = append(out, gv.Version)
				}
			}
		}
	}
	if len(out) > 1 {
		sort.Slice(out, func(i, j int) bool {
			return apiversion.MustCompare(out[i], out[j]) > 0
		})
	}
	return out, nil
}

func ListKinds2(kc client.Client) error {
	rs, err := kc.RESTMapper().ResourcesFor(schema.GroupVersionResource{
		Group:    "kubedb.com",
		Version:  "",
		Resource: "",
	})
	if err != nil {
		return err
	}
	fmt.Println(rs)

	kinds, err := kc.RESTMapper().KindsFor(schema.GroupVersionResource{
		Group:    "kubedb.com",
		Version:  "",
		Resource: "",
	})
	if err != nil {
		return err
	}
	for _, k := range kinds {
		fmt.Println(k)
	}

	mappings, err := kc.RESTMapper().RESTMappings(schema.GroupKind{
		Group: "kubedb.com",
		Kind:  "",
	})
	if err != nil {
		return err
	}
	for _, mapping := range mappings {
		fmt.Println(mapping.GroupVersionKind)
	}

	return nil
}
