package main

import (
	"fmt"
	"sigs.k8s.io/yaml"
)

func main() {
	y1 := `host: go.bytebuilders.dev

paths:
  /appcatalog:
    repo: https://github.com/bytebuilders/appcatalog
  /ace:
    repo: https://github.com/bytebuilders/ace
  /audit:
    repo: https://github.com/bytebuilders/audit
  /b3:
    repo: https://github.com/bytebuilders/b3
  /capi-config:
    repo: https://github.com/bytebuilders/capi-config
  /capa-vpc-peering-operator:
    repo: https://github.com/bytebuilders/capa-vpc-peering-operator
  /lib-selfhost:
    repo: https://github.com/bytebuilders/lib-selfhost
  /cert-manager-webhook-ace:
    repo: https://github.com/bytebuilders/cert-manager-webhook-ace
  /cloudflare-dns-proxy:
    repo: https://github.com/bytebuilders/cloudflare-dns-proxy
  /smtprelay:
    repo: https://github.com/bytebuilders/smtprelay
  /client:
    repo: https://github.com/bytebuilders/client-go
  /configurator:
    repo: https://github.com/bytebuilders/configurator
  /installer:
    repo: https://github.com/bytebuilders/installer
  /kube-auth-manager:
    repo: https://github.com/bytebuilders/kube-auth-manager
  /kube-bind-server:
    repo: https://github.com/bytebuilders/kube-bind-server
  /license-proxyserver:
    repo: https://github.com/bytebuilders/license-proxyserver
  /license-tester:
    repo: https://github.com/bytebuilders/license-tester
  /license-verifier:
    repo: https://github.com/bytebuilders/license-verifier
    versions:
      - kubernetes
  /nats-logger:
    repo: https://github.com/bytebuilders/nats-logger
  /offline-license-server:
    repo: https://github.com/bytebuilders/offline-license-server
  /products:
    repo: https://github.com/bytebuilders/products
  /resource-model:
    repo: https://github.com/bytebuilders/resource-model
  /uibuilder-tools:
    repo: https://github.com/bytebuilders/uibuilder-tools
  /ui-wizards:
    repo: https://github.com/bytebuilders/ui-wizards
`

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
