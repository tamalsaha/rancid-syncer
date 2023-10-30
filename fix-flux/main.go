package main

import (
	"fmt"
	"sigs.k8s.io/yaml"
)

func main() {
	y1 := `    ace:
      enabled: false
      version: "v2023.10.18"
    cert-manager-csi-driver-cacerts:
      enabled: true
      version: "v2023.10.1"
    cert-manager-webhook-ace:
      enabled: true
      version: "v2023.10.18"
    cert-manager:
      enabled: true
      version: "v1.11.0"
      values: # +doc-gen:break
        installCRDs: true
    kubedb:
      enabled: true
      version: "v2023.10.26-rc.0"
      values: # +doc-gen:break
        kubedb-provisioner:
          enabled: true
        kubedb-catalog:
          enabled: true
        kubedb-ops-manager:
          enabled: true
        kubedb-autoscaler:
          enabled: false
        kubedb-dashboard:
          enabled: false
        kubedb-schema-manager:
          enabled: false
        kubedb-metrics:
          enabled: false
    docker-machine-operator:
      enabled: true
      version: "v2023.10.18"
    external-dns-operator:
      enabled: true
      version: "v2023.10.1"
    kube-ui-server:
      enabled: true
      version: "v2023.10.1"
    license-proxyserver:
      enabled: true
      version: "v2023.10.18"
    reloader:
      enabled: true
      version: "v1.0.24"
    kube-prometheus-stack:
      enabled: true
      version: "52.1.0"
    opscenter-features:
      enabled: true
      version: "v2023.10.18"
    panopticon:
      enabled: true
      version: "v2023.10.1"
      values: # +doc-gen:break
        monitoring:
          enabled: true
          agent: prometheus.io/operator
          serviceMonitor:
            labels:
              release: kube-prometheus-stack
    stash:
      enabled: true
      version: "v2023.10.9"
      values: # +doc-gen:break
        features:
          enterprise: true`

	var obj map[string]any
	err := yaml.Unmarshal([]byte(y1), &obj)
	if err != nil {
		panic(err)
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
}
