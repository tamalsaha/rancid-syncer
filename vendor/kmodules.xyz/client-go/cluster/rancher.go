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

package cluster

import (
	"context"
	"sort"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const LabelKeyRancherProjectId = "field.cattle.io/projectId"

func IsRancherManaged(mapper meta.RESTMapper) bool {
	if _, err := mapper.RESTMappings(schema.GroupKind{
		Group: "management.cattle.io",
		Kind:  "Cluster",
	}); err == nil {
		return true
	}
	return false
}

func IsInDefaultProject(kc client.Client, nsName string) (bool, error) {
	return isInProject(kc, nsName, metav1.NamespaceDefault)
}

func IsInSystemProject(kc client.Client, nsName string) (bool, error) {
	return isInProject(kc, nsName, metav1.NamespaceSystem)
}

func IsInUserProject(kc client.Client, nsName string) (bool, error) {
	isDefault, err := IsInDefaultProject(kc, nsName)
	if err != nil {
		return false, err
	}
	isSys, err := IsInSystemProject(kc, nsName)
	if err != nil {
		return false, err
	}
	return !isDefault && !isSys, nil
}

func isInProject(kc client.Client, nsName, seedNS string) (bool, error) {
	if nsName == seedNS {
		return true, nil
	}

	var ns core.Namespace
	err := kc.Get(context.TODO(), client.ObjectKey{Name: nsName}, &ns)
	if err != nil {
		return false, err
	}
	projectId, exists := ns.Labels[LabelKeyRancherProjectId]
	if !exists {
		return false, nil
	}

	seedProjectId, _, err := GetProjectId(kc, seedNS)
	if err != nil {
		return false, err
	}
	return projectId == seedProjectId, nil
}

func GetDefaultProjectId(kc client.Client) (string, bool, error) {
	return GetProjectId(kc, metav1.NamespaceDefault)
}

func GetSystemProjectId(kc client.Client) (string, bool, error) {
	return GetProjectId(kc, metav1.NamespaceSystem)
}

func GetProjectId(kc client.Client, nsName string) (string, bool, error) {
	var ns core.Namespace
	err := kc.Get(context.TODO(), client.ObjectKey{Name: nsName}, &ns)
	if err != nil {
		return "", false, err
	}
	projectId, found := ns.Labels[LabelKeyRancherProjectId]
	return projectId, found, nil
}

func ListSiblingNamespaces(kc client.Client, nsName string) ([]string, error) {
	projectId, found, err := GetProjectId(kc, nsName)
	if err != nil || !found {
		return nil, err
	}
	return ListProjectNamespaces(kc, projectId)
}

func ListProjectNamespaces(kc client.Client, projectId string) ([]string, error) {
	var list core.NamespaceList
	err := kc.List(context.TODO(), &list, client.MatchingLabels{
		LabelKeyRancherProjectId: projectId,
	})
	if err != nil {
		return nil, err
	}
	namespaces := make([]string, 0, len(list.Items))
	for _, x := range list.Items {
		namespaces = append(namespaces, x.Name)
	}
	sort.Slice(namespaces, func(i, j int) bool {
		return namespaces[i] < namespaces[j]
	})
	return namespaces, nil
}
