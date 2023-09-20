package main

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	kmapi "kmodules.xyz/client-go/api/v1"
	clustermanger "kmodules.xyz/client-go/cluster"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
	"sort"
	chartsapi "x-helm.dev/apimachinery/apis/charts/v1alpha1"
)

type PresetQuery struct {
	metav1.TypeMeta `json:",inline"`
	// Request describes the attributes for the graph request.
	// +optional
	Request *chartsapi.ChartPresetFlatRef `json:"request,omitempty"`
	// Response describes the attributes for the graph response.
	// +optional
	Response []ChartPresetValues `json:"response,omitempty"`
}

type ChartPresetValues struct {
	Source rsapi.SourceLocator   `json:"source"`
	Values *runtime.RawExtension `json:"values"`
}

func NewClient() (*rest.Config, versioned.Interface, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
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

	projects, err := clustermanger.ListRancherProjects(kc)
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

func MergePresetValues(kc client.Client, ref chartsapi.ChartPresetFlatRef) ([]ChartPresetValues, error) {
	var values []ChartPresetValues

	if ref.PresetName != "" {
		rid := &kmapi.ResourceID{
			Group: ref.Group,
			Name:  ref.Resource,
			Kind:  ref.Kind,
		}
		rid, err := kmapi.ExtractResourceID(kc.RESTMapper(), *rid)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to detect resource ID for %#v", *rid)
		}
		ed, ok := resourceeditors.LoadByGVR(kc, rid.GroupVersionResource())
		if !ok {
			return nil, errors.Errorf("failed to detect ResourceEditor for %#v", rid.GroupVersionResource())
		}

		for _, variant := range ed.Spec.Variants {
			if variant.Name != ref.PresetName {
				continue
			}
			if variant.Selector == nil {
				continue
			}

			sel, err := metav1.LabelSelectorAsSelector(variant.Selector)
			if err != nil {
				return nil, err
			}
			var list chartsapi.ClusterChartPresetList
			err = kc.List(context.TODO(), &list, client.MatchingLabelsSelector{Selector: sel})
			if err != nil {
				return nil, err
			}
			ccps := list.Items
			sort.Slice(ccps, func(i, j int) bool {
				return ccps[i].Name < ccps[j].Name
			})
			for _, ccp := range ccps {
				if ref.Namespace == "" {
					values = append(values, ChartPresetValues{
						Source: rsapi.SourceLocator{
							Resource: kmapi.ResourceID{
								Group:   chartsapi.GroupVersion.Group,
								Version: chartsapi.GroupVersion.Version,
								Kind:    chartsapi.ResourceKindClusterChartPreset,
							},
							Ref: kmapi.ObjectReference{
								Namespace: ccp.Namespace,
								Name:      ccp.Namespace,
							},
						},
						Values: ccp.Spec.Values,
					})
				} else {
					var cp chartsapi.ChartPreset
					err := kc.Get(context.TODO(), client.ObjectKey{Namespace: ref.Namespace, Name: ccp.Name}, &cp)
					if err == nil {
						values = append(values, ChartPresetValues{
							Source: rsapi.SourceLocator{
								Resource: kmapi.ResourceID{
									Group:   chartsapi.GroupVersion.Group,
									Version: chartsapi.GroupVersion.Version,
									Kind:    chartsapi.ResourceKindChartPreset,
								},
								Ref: kmapi.ObjectReference{
									Namespace: cp.Namespace,
									Name:      cp.Namespace,
								},
							},
							Values: cp.Spec.Values,
						})
					} else if apierrors.IsNotFound(err) {
						values = append(values, ChartPresetValues{
							Source: rsapi.SourceLocator{
								Resource: kmapi.ResourceID{
									Group:   chartsapi.GroupVersion.Group,
									Version: chartsapi.GroupVersion.Version,
									Kind:    chartsapi.ResourceKindClusterChartPreset,
								},
								Ref: kmapi.ObjectReference{
									Namespace: ccp.Namespace,
									Name:      ccp.Namespace,
								},
							},
							Values: ccp.Spec.Values,
						})
					} else {
						return nil, err
					}
				}
			}
		}
	}
	return values, nil
}
