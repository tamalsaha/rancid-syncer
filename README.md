# rancid-syncer

<img src="hero.png" />

## Generate apis

```bash
> kubebuilder init --domain k8s.appscode.com --skip-go-version-check
> kubebuilder edit --multigroup=true
> kubebuilder create api --group management --version v1alpha1 --kind Project --namespaced=false
```

## Rancher Monitoring

- rancher-monitoring from Cluster Tools
- Prometheus Federator
- Add labels to cluster AlertManager and Prometheus

- https://ranchermanager.docs.rancher.com/how-to-guides/advanced-user-guides/monitoring-alerting-guides/enable-monitoring#install-the-monitoring-application

- https://ranchermanager.docs.rancher.com/how-to-guides/advanced-user-guides/monitoring-alerting-guides/prometheus-federator-guides/enable-prometheus-federator

## Resource Quota

Annotation on Project in the app cluster

```
metadata:
  annotations:
    MEMORY_LIMIT_GB: '32'
    STORAGE_LIMIT_GB: '200'
    TCP_PORT_RANGE: 50000-50014
```

## Trickster

{uid}-{cluster-uid}
|
|
V
{uid}.{cluster-uid}.{projctId}

/register/

/{uid}-{cluster-uid}/

Data Source {cluster-name}-{projctId}

## Service rbac

```
prometheus.prometheusSpec.ignoreNamespaceSelectors
```
ignoreNamespaceSelectors is always `true` for Project Promethues. So, we have to create fake Service without labels to work around it.


- https://github.com/prometheus-operator/prometheus-operator/issues/5386

- https://github.com/prometheus-operator/prometheus-operator/blob/7aa85a6f94dc5dbb41ba48fc27d9a255594e9e49/pkg/prometheus/promcfg.go#L1529-L1569

- https://github.com/prometheus/prometheus/blob/86729d4d7b8659e2b90fa65ae2d42ecddc3657bc/docs/configuration/configuration.md?plain=1#L2150-L2178

- https://github.com/prometheus/prometheus/blob/86729d4d7b8659e2b90fa65ae2d42ecddc3657bc/discovery/kubernetes/kubernetes.go#L253

- https://github.com/prometheus/prometheus/blob/86729d4d7b8659e2b90fa65ae2d42ecddc3657bc/discovery/kubernetes/kubernetes.go#L201

- `External Name` service will not work
https://github.com/prometheus-operator/prometheus-operator/issues/3020

