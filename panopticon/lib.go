package panopticon

import (
	"context"
	"fmt"
	"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	cu "kmodules.xyz/client-go/client"
	meta_util "kmodules.xyz/client-go/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

/*
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  annotations:
    meta.helm.sh/release-name: panopticon
    meta.helm.sh/release-namespace: monitoring
  creationTimestamp: "2023-09-18T02:17:23Z"
  generation: 1
  labels:
    app.kubernetes.io/managed-by: Helm
    helm.toolkit.fluxcd.io/name: panopticon
    helm.toolkit.fluxcd.io/namespace: kubeops
    release: kube-prometheus-stack
  name: panopticon
  namespace: monitoring
  resourceVersion: "3743"
  uid: 40d07af0-f4e1-4f9f-9e6e-4bf322b01bdf
spec:
  endpoints:
  - bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    interval: 10s
    port: api
    relabelings:
    - action: labeldrop
      regex: (pod|service|endpoint|namespace)
    scheme: https
    tlsConfig:
      ca:
        secret:
          key: tls.crt
          name: panopticon-apiserver-cert
      serverName: panopticon.monitoring.svc
  - bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    interval: 10s
    port: telemetry
    scheme: http
  namespaceSelector:
    matchNames:
    - monitoring
  selector:
    matchLabels:
      app.kubernetes.io/instance: panopticon
      app.kubernetes.io/name: panopticon
*/

func CreateServiceMonitor(kc client.Client, prom *monitoringv1.Prometheus, panopticon *core.Service) (*monitoringv1.ServiceMonitor, error) {
	svcmon := monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      panopticon.Name,
			Namespace: prom.Namespace,
		},
	}

	promGVK := schema.GroupVersionKind{
		Group:   monitoring.GroupName,
		Version: monitoringv1.Version,
		Kind:    "Prometheus",
	}

	svcmonLabels, ok := meta_util.LabelsForLabelSelector(prom.Spec.ServiceMonitorSelector)
	if !ok {
		klog.Warningln("Prometheus %s/%s uses match expressions in ServiceMonitorSelector", prom.Namespace, prom.Name)
	}

	vt, err := cu.CreateOrPatch(context.TODO(), kc, &svcmon, func(in client.Object, createOp bool) client.Object {
		obj := in.(*monitoringv1.ServiceMonitor)

		obj.Labels = meta_util.OverwriteKeys(obj.Labels, svcmonLabels)
		ref := metav1.NewControllerRef(prom, promGVK)
		obj.OwnerReferences = []metav1.OwnerReference{*ref}

		obj.Spec.NamespaceSelector = monitoringv1.NamespaceSelector{
			MatchNames: []string{
				panopticon.Namespace,
			},
		}
		obj.Spec.Selector = metav1.LabelSelector{
			MatchLabels: panopticon.Labels,
		}

		obj.Spec.Endpoints = []monitoringv1.Endpoint{
			{
				Scheme:          "https",
				Port:            "api",
				Interval:        "10s",
				BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				TLSConfig: &monitoringv1.TLSConfig{
					SafeTLSConfig: monitoringv1.SafeTLSConfig{
						CA: monitoringv1.SecretOrConfigMap{
							Secret: &core.SecretKeySelector{
								LocalObjectReference: core.LocalObjectReference{
									Name: fmt.Sprintf("%s-apiserver-cert", panopticon.Name),
								},
								Key: "tls.crt",
							},
						},
						ServerName: fmt.Sprintf("%s.%s.svc", panopticon.Name, panopticon.Namespace),
					},
				},
				RelabelConfigs: []*monitoringv1.RelabelConfig{
					{},
				},
			},
			{
				Scheme:          "http",
				Port:            "telemetry",
				Interval:        "10s",
				BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
			},
		}

		return obj
	})
	if err != nil {
		return nil, err
	}
	klog.Infof("%s ServiceMonitor %s/%s", vt, svcmon.Namespace, svcmon.Name)

	return &svcmon, nil
}
