package main

import (
	"context"
	"encoding/json"
	"fmt"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	kutil "kmodules.xyz/client-go"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

func main() {
	ctrl.SetLogger(klogr.New())
	if err := useGeneratedClient(); err != nil {
		panic(err)
	}
	time.Sleep(1 * time.Minute)
}

func useGeneratedClient() error {
	fmt.Println("Using Generated client")
	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = 100
	cfg.Burst = 100

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	p, err := kc.CoreV1().Pods("default").Get(context.TODO(), "ha-postgres-0", metav1.GetOptions{})
	if err != nil {
		return err
	}

	u, kt, err := PatchPod(context.TODO(), kc, p, func(in *core.Pod) *core.Pod {
		in.Labels["otel"] = "abc2"
		in.Labels["wasm"] = "omg"
		return in
	}, metav1.PatchOptions{})
	if err != nil {
		return err
	}

	fmt.Println("kt =", kt)
	fmt.Println("pod.Name =", u.Name)

	return nil
}

func PatchPod(ctx context.Context, c kubernetes.Interface, cur *core.Pod, transform func(*core.Pod) *core.Pod, opts metav1.PatchOptions) (*core.Pod, kutil.VerbType, error) {
	return PatchPodObject(ctx, c, cur, transform(cur.DeepCopy()), opts)
}

func PatchPodObject(ctx context.Context, c kubernetes.Interface, cur, mod *core.Pod, opts metav1.PatchOptions) (*core.Pod, kutil.VerbType, error) {
	curJson, err := json.Marshal(cur)
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}

	modJson, err := json.Marshal(mod)
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(curJson, modJson, core.Pod{})
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}
	if len(patch) == 0 || string(patch) == "{}" {
		return cur, kutil.VerbUnchanged, nil
	}
	klog.V(3).Infof("Patching Pod %s/%s with %s", cur.Namespace, cur.Name, string(patch))
	out, err := c.CoreV1().Pods(cur.Namespace).Patch(ctx, cur.Name, types.StrategicMergePatchType, patch, opts)
	return out, kutil.VerbPatched, err
}
