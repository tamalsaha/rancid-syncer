package main

import (
	"context"
	"fmt"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	"kmodules.xyz/resource-metadata/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"strings"
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
	_, _, kc, err := NewClient()
	if err != nil {
		return err
	}

	vcluster, err := IsVCluster(kc)
	if err != nil {
		return err
	}
	fmt.Println(vcluster)

	return nil
}

/*
	vcluster.loft.sh/managed-labels: |-

status:

	addresses:
	- address: kind-control-plane.nodes.vcluster.com
	  type: Hostname
	- address: 10.96.163.162
	  type: InternalIP
*/
func IsVCluster(kc client.Client) (bool, error) {
	var list core.NodeList
	err := kc.List(context.TODO(), &list)
	if err != nil {
		return false, err
	}
	for _, node := range list.Items {
		_, f1 := node.Annotations["vcluster.loft.sh/managed-annotations"]
		_, f2 := node.Annotations["vcluster.loft.sh/managed-labels"]
		for _, addr := range node.Status.Addresses {
			if addr.Type == core.NodeHostName {
				if f1 && f2 && strings.HasSuffix(addr.Address, ".nodes.vcluster.com") {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
