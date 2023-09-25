/*
Copyright AppsCode Inc. and Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package versioned

import (
	"fmt"
	"net/http"

	discovery "k8s.io/client-go/discovery"
	rest "k8s.io/client-go/rest"
	flowcontrol "k8s.io/client-go/util/flowcontrol"
	corev1alpha1 "kmodules.xyz/resource-metadata/client/clientset/versioned/typed/core/v1alpha1"
	managementv1alpha1 "kmodules.xyz/resource-metadata/client/clientset/versioned/typed/management/v1alpha1"
	metav1alpha1 "kmodules.xyz/resource-metadata/client/clientset/versioned/typed/meta/v1alpha1"
	uiv1alpha1 "kmodules.xyz/resource-metadata/client/clientset/versioned/typed/ui/v1alpha1"
)

type Interface interface {
	Discovery() discovery.DiscoveryInterface
	CoreV1alpha1() corev1alpha1.CoreV1alpha1Interface
	ManagementV1alpha1() managementv1alpha1.ManagementV1alpha1Interface
	MetaV1alpha1() metav1alpha1.MetaV1alpha1Interface
	UiV1alpha1() uiv1alpha1.UiV1alpha1Interface
}

// Clientset contains the clients for groups. Each group has exactly one
// version included in a Clientset.
type Clientset struct {
	*discovery.DiscoveryClient
	coreV1alpha1       *corev1alpha1.CoreV1alpha1Client
	managementV1alpha1 *managementv1alpha1.ManagementV1alpha1Client
	metaV1alpha1       *metav1alpha1.MetaV1alpha1Client
	uiV1alpha1         *uiv1alpha1.UiV1alpha1Client
}

// CoreV1alpha1 retrieves the CoreV1alpha1Client
func (c *Clientset) CoreV1alpha1() corev1alpha1.CoreV1alpha1Interface {
	return c.coreV1alpha1
}

// ManagementV1alpha1 retrieves the ManagementV1alpha1Client
func (c *Clientset) ManagementV1alpha1() managementv1alpha1.ManagementV1alpha1Interface {
	return c.managementV1alpha1
}

// MetaV1alpha1 retrieves the MetaV1alpha1Client
func (c *Clientset) MetaV1alpha1() metav1alpha1.MetaV1alpha1Interface {
	return c.metaV1alpha1
}

// UiV1alpha1 retrieves the UiV1alpha1Client
func (c *Clientset) UiV1alpha1() uiv1alpha1.UiV1alpha1Interface {
	return c.uiV1alpha1
}

// Discovery retrieves the DiscoveryClient
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

// NewForConfig creates a new Clientset for the given config.
// If config's RateLimiter is not set and QPS and Burst are acceptable,
// NewForConfig will generate a rate-limiter in configShallowCopy.
// NewForConfig is equivalent to NewForConfigAndClient(c, httpClient),
// where httpClient was generated with rest.HTTPClientFor(c).
func NewForConfig(c *rest.Config) (*Clientset, error) {
	configShallowCopy := *c

	if configShallowCopy.UserAgent == "" {
		configShallowCopy.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	// share the transport between all clients
	httpClient, err := rest.HTTPClientFor(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	return NewForConfigAndClient(&configShallowCopy, httpClient)
}

// NewForConfigAndClient creates a new Clientset for the given config and http client.
// Note the http client provided takes precedence over the configured transport values.
// If config's RateLimiter is not set and QPS and Burst are acceptable,
// NewForConfigAndClient will generate a rate-limiter in configShallowCopy.
func NewForConfigAndClient(c *rest.Config, httpClient *http.Client) (*Clientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		if configShallowCopy.Burst <= 0 {
			return nil, fmt.Errorf("burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0")
		}
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}

	var cs Clientset
	var err error
	cs.coreV1alpha1, err = corev1alpha1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.managementV1alpha1, err = managementv1alpha1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.metaV1alpha1, err = metav1alpha1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	cs.uiV1alpha1, err = uiv1alpha1.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}

	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a new Clientset for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	cs, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return cs
}

// New creates a new Clientset for the given RESTClient.
func New(c rest.Interface) *Clientset {
	var cs Clientset
	cs.coreV1alpha1 = corev1alpha1.New(c)
	cs.managementV1alpha1 = managementv1alpha1.New(c)
	cs.metaV1alpha1 = metav1alpha1.New(c)
	cs.uiV1alpha1 = uiv1alpha1.New(c)

	cs.DiscoveryClient = discovery.NewDiscoveryClient(c)
	return &cs
}
