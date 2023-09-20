package main

import (
	"context"
	"fmt"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	clustermeta "kmodules.xyz/client-go/cluster"
	uiv1alpha1 "kmodules.xyz/resource-metadata/apis/ui/v1alpha1"
	"sort"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	kmapi "kmodules.xyz/client-go/api/v1"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	"kmodules.xyz/resource-metadata/hub/resourceeditors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/yaml"
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
	_, _, kc, err := NewClient()
	if err != nil {
		return err
	}

	presets, err := MergePresetValues(kc, chartsapi.ChartPresetFlatRef{})
	data, err := yaml.Marshal(presets)
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	return nil
}

func LoadPresetValues(kc client.Client, ref chartsapi.ChartPresetFlatRef) ([]ChartPresetValues, error) {
	if ref.PresetName == "" {
		return nil, errors.New("preset name is not set")
	}

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

	var variant *uiv1alpha1.VariantRef
	for i := range ed.Spec.Variants {
		if ed.Spec.Variants[i].Name == ref.PresetName {
			variant = &ed.Spec.Variants[i]
			break
		}
	}
	if variant == nil {
		return nil, errors.Errorf("No variant with name %s found for %+v", ref.PresetName, *rid)
	}

	if variant.Selector == nil {
		return nil, nil // ERROR?
	}
	sel, err := metav1.LabelSelectorAsSelector(variant.Selector)
	if err != nil {
		return nil, err
	}

	if ref.Namespace == "" {
		return bundleClusterChartPresets(kc, sel, nil)
	}

	values, err := bundleChartPresets(kc, ref.Namespace, sel, nil)
	if err != nil {
		return nil, err
	}
	knownPresets := map[string]bool{} // true => cp, false => ccp
	for _, v := range values {
		knownPresets[v.Source.Ref.Name] = true
	}

	if clustermeta.IsRancherManaged(kc.RESTMapper()) {
		var ns core.Namespace
		err = kc.Get(context.TODO(), client.ObjectKey{Name: ref.Namespace}, &ns)
		if err != nil {
			return nil, err
		}
		projectId, found := ns.Labels[clustermeta.LabelKeyRancherProjectId]
		if !found {
			// NS not in a project. So, just add the extra CCPs
			ccps, err := bundleClusterChartPresets(kc, sel, knownPresets)
			if err != nil {
				return nil, err
			}
			values = append(ccps, values...)
			return values, nil
		}

		var nsList core.NamespaceList
		err := kc.List(context.TODO(), &nsList, client.MatchingLabels{
			clustermeta.LabelKeyRancherProjectId: projectId,
		})
		if err != nil {
			return nil, err
		}
		namespaces := nsList.Items
		sort.Slice(namespaces, func(i, j int) bool {
			return namespaces[i].Name < namespaces[j].Name
		})

		projectPresets := map[string]bool{} // true => cp, false => ccp
		for i := len(namespaces) - 1; i >= 0; i-- {
			if namespaces[i].Name == ref.Namespace {
				continue
			}

			nsPresets, err := bundleChartPresets(kc, namespaces[i].Name, sel, knownPresets)
			if err != nil {
				return nil, err
			}
			for _, v := range nsPresets {
				projectPresets[v.Source.Ref.Name] = true
			}
			values = append(nsPresets, values...)
		}
		// mark project presets as known
		for k, v := range projectPresets {
			knownPresets[k] = v
		}

		ccps, err := bundleClusterChartPresets(kc, sel, knownPresets)
		if err != nil {
			return nil, err
		}
		values = append(ccps, values...)
	} else {
		ccps, err := bundleClusterChartPresets(kc, sel, knownPresets)
		if err != nil {
			return nil, err
		}
		values = append(ccps, values...)
	}

	return values, nil
}

func bundleChartPresets(kc client.Client, ns string, sel labels.Selector, knownPresets map[string]bool) ([]ChartPresetValues, error) {
	var list chartsapi.ChartPresetList
	err := kc.List(context.TODO(), &list, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: sel})
	if err != nil {
		return nil, err
	}
	cps := list.Items
	sort.Slice(cps, func(i, j int) bool {
		return cps[i].Name < cps[j].Name
	})

	values := make([]ChartPresetValues, 0, len(cps))
	for _, cp := range cps {
		if _, exists := knownPresets[cp.Name]; exists {
			continue
		}

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
	}
	return values, err
}

func bundleClusterChartPresets(kc client.Client, sel labels.Selector, knownPresets map[string]bool) ([]ChartPresetValues, error) {
	var list chartsapi.ClusterChartPresetList
	err := kc.List(context.TODO(), &list, client.MatchingLabelsSelector{Selector: sel})
	if err != nil {
		return nil, err
	}

	ccps := list.Items
	sort.Slice(ccps, func(i, j int) bool {
		return ccps[i].Name < ccps[j].Name
	})

	values := make([]ChartPresetValues, 0, len(ccps))
	for _, ccp := range ccps {
		if _, exists := knownPresets[ccp.Name]; exists {
			continue
		}

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
	}
	return values, nil
}

// unused
func MergePresetValues(kc client.Client, ref chartsapi.ChartPresetFlatRef) ([]ChartPresetValues, error) {
	if ref.Namespace != "" {
		return nil, errors.New("Only call when namespace is not known")
	}

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
