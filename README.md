# rancid-syncer

<img src="hero.png" />

## Generate apis

```bash
> kubebuilder init --domain k8s.appscode.com --skip-go-version-check
> kubebuilder edit --multigroup=true
> kubebuilder create api --group management --version v1alpha1 --kind Project --namespaced=false
```
