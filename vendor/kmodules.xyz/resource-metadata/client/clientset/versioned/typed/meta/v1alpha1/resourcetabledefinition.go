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

package v1alpha1

import (
	"context"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rest "k8s.io/client-go/rest"
	v1alpha1 "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	scheme "kmodules.xyz/resource-metadata/client/clientset/versioned/scheme"
)

// ResourceTableDefinitionsGetter has a method to return a ResourceTableDefinitionInterface.
// A group's client should implement this interface.
type ResourceTableDefinitionsGetter interface {
	ResourceTableDefinitions() ResourceTableDefinitionInterface
}

// ResourceTableDefinitionInterface has methods to work with ResourceTableDefinition resources.
type ResourceTableDefinitionInterface interface {
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.ResourceTableDefinition, error)
	ResourceTableDefinitionExpansion
}

// resourceTableDefinitions implements ResourceTableDefinitionInterface
type resourceTableDefinitions struct {
	client rest.Interface
}

// newResourceTableDefinitions returns a ResourceTableDefinitions
func newResourceTableDefinitions(c *MetaV1alpha1Client) *resourceTableDefinitions {
	return &resourceTableDefinitions{
		client: c.RESTClient(),
	}
}

// Get takes name of the resourceTableDefinition, and returns the corresponding resourceTableDefinition object, and an error if there is any.
func (c *resourceTableDefinitions) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ResourceTableDefinition, err error) {
	result = &v1alpha1.ResourceTableDefinition{}
	err = c.client.Get().
		Resource("resourcetabledefinitions").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}
