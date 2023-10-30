package main

import (
	"encoding/json"
	"errors"
	"fmt"
	jp "github.com/evanphx/json-patch"
	"gomodules.xyz/jsonpatch/v2"
	"sigs.k8s.io/yaml"
)

func getOpscenterFeaturesPatchValues() ([]byte, error) {
	patchValues := make(map[string]interface{})

	patchValues["capi"] = map[string]interface{}{
		"provider":    "opts.clusterInfo.Status.ClusterAPI.Provider",
		"namespace":   "opts.clusterInfo.Status.ClusterAPI.Namespace",
		"clusterName": "opts.clusterInfo.Status.ClusterAPI.ClusterName",
	}

	patchValues["clusterManagers"] = []string{"rancher", "ace"}

	//patchValues["image"] = preset.Image
	//patchValues["registry"] = preset.Registry

	/*
		helm:
		  repositories:
		    # oci://harbor.appscode.ninja/ac/appscode-charts
		    appscode-charts-oci:
		      url: oci://ghcr.io/appscode-charts

		  releases:
		    aws-ebs-csi-driver:
		      version: "2.23.0"
	*/
	patchValues["helm"] = map[string]any{
		"repositories": map[string]any{
			"appscode-charts-oci": map[string]any{
				"url": "oci://ghcr.io/appscode-charts",
			},
		},
	}

	patch, err := CreateJsonPatch(map[string]interface{}{
		"helm": map[string]any{
			"repositories": map[string]any{},
		},
	}, patchValues)
	if err != nil {
		return nil, err
	}

	return patch, nil
}

func CreateJsonPatch(empty, custom interface{}) ([]byte, error) {
	emptyBytes, err := json.Marshal(empty)
	if err != nil {
		return nil, err
	}
	customBytes, err := json.Marshal(custom)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.CreatePatch(emptyBytes, customBytes)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(patch, "", "  ")
}

var base = `# Default values for opscenter-features.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

nameOverride: ""
fullnameOverride: ""

image:
  registryFQDN: ""
  proxies:
    # r.appscode.com
    appscode: r.appscode.com
    # company/bin:tag
    dockerHub: ""
    # alpine, nginx etc.
    dockerLibrary: ""
    # ghcr.io/company/bin:tag
    ghcr: ghcr.io
    # quay.io/company/bin:tag
    quay: quay.io
    # registry.k8s.io/bin:tag
    kubernetes: registry.k8s.io

# image:
#   registryFQDN: harbor.appscode.ninja
#   proxies:
#     dockerHub: harbor.appscode.ninja/dockerhub
#     dockerLibrary: ""
#     ghcr: harbor.appscode.ninja/ghcr
#     quay: harbor.appscode.ninja/quay
#     kubernetes: harbor.appscode.ninja/k8s
#     appscode: harbor.appscode.ninja/ac

registry:
  credentials: {}
  #   username: "abc"
  #   password: "xyz"

helm:
  repositories:
    # oci://harbor.appscode.ninja/ac/appscode-charts
    appscode-charts-oci:
      url: oci://ghcr.io/appscode-charts

  releases:
    aws-ebs-csi-driver:
      version: "2.23.0"
    capa-vpc-peering-operator:
      version: "v2023.10.18"
    capi-cluster-presets:
      version: "v2023.10.18"
    cert-manager:
      version: "v1.11.0"
    cluster-autoscaler:
      version: "9.29.0"
    crossplane:
      version: "1.13.2"
    external-dns-operator:
      version: "v2023.10.1"
    falco:
      version: "3.8.4"
    falco-ui-server:
      version: "v2023.10.1"
    flux2:
      version: "2.10.6"
    gatekeeper:
      version: "3.13.3"
    gatekeeper-grafana-dashboards:
      version: "v2023.10.1"
    gatekeeper-library:
      version: "v2023.10.1"
    gateway-helm:
      version: "v0.0.0-latest"
    grafana-operator:
      version: "v0.0.3"
    keda:
      version: "2.12.0"
    keda-add-ons-http:
      version: "0.6.0"
    kube-grafana-dashboards:
      version: "v2023.10.1"
    kube-prometheus-stack:
      version: "52.1.0"
    kube-ui-server:
      version: "v2023.10.1"
    kubedb:
      version: "v2023.10.26-rc.0"
    kubedb-opscenter:
      version: "v2023.10.26-rc.0"
    kubeform-provider-aws:
      version: "v2023.06.27"
    kubeform-provider-azure:
      version: "v2023.06.27"
    kubeform-provider-gcp:
      version: "v2023.06.27"
    kubestash:
      version: "v2023.04.14"
    kubestash-presets:
      version: "v2023.10.18"
    kubevault:
      version: "v2023.10.26-rc.0"
    kubevault-opscenter:
      version: "v2023.10.26-rc.0"
    license-proxyserver:
      version: "v2023.10.18"
    monitoring-operator:
      version: "v0.0.3"
    multicluster-controlplane:
      version: "0.2.0"
    opencost:
      version: "1.18.1"
    opscenter-features:
      version: "v2023.10.18"
    panopticon:
      version: "v2023.10.1"
    scanner:
      version: "v2023.10.18"
    stash:
      version: "v2023.10.9"
    stash-opscenter:
      version: "v2023.10.9"
    stash-presets:
      version: "v2023.10.18"
    supervisor:
      version: "v2023.10.1"
    voyager:
      version: "v2023.9.18"

clusterManagers: []

capi:
  provider: ""
  namespace: ""
  clusterName: ""
`

func applyPatch(p2 []byte) error {
	patch, err := jp.DecodePatch(p2)
	if err != nil {
		return err
	}

	baseBytes, err := yaml.YAMLToJSON([]byte(base))
	if err != nil {
		return errors.New("failed to convert values file")
	}
	valuesBytes, err := patch.Apply(baseBytes)
	if err != nil {
		return err
	}

	fmt.Println(string(valuesBytes))

	return nil
}

func main() {
	patch, err := getOpscenterFeaturesPatchValues()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(patch))
	fmt.Println("-----------------------")

	err = applyPatch(patch)
	if err != nil {
		panic(err)
	}
}
