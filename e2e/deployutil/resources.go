// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package deployutil

import (
	"os"
	"path/filepath"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	v1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const (
	crdsManifestDirectory     = "../cmd/operator/deploy/crds"
	operatorManifestDirectory = "../cmd/operator/deploy/operator"
)

func crdResources(scheme *runtime.Scheme, restMapper meta.RESTMapper) ([]client.Object, error) {
	var resources []client.Object
	files, err := os.ReadDir(crdsManifestDirectory)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		objs, err := kubeutil.ResourcesFromFile(scheme, filepath.Join(crdsManifestDirectory, file.Name()))
		if err != nil {
			return nil, err
		}
		resources = append(resources, objs...)
	}
	return resources, nil
}

func operatorResources(scheme *runtime.Scheme, restMapper meta.RESTMapper) (globalResources, localResources []client.Object, _ error) {
	files, err := os.ReadDir(operatorManifestDirectory)
	if err != nil {
		return nil, nil, err
	}
	for _, file := range files {
		objs, err := kubeutil.ResourcesFromFile(scheme, filepath.Join(operatorManifestDirectory, file.Name()))
		if err != nil {
			return nil, nil, err
		}
		for _, obj := range objs {
			namespaced, err := apiutil.IsObjectNamespaced(obj, scheme, restMapper)
			if err != nil {
				return nil, nil, err
			}
			if namespaced || isMetaNamespaced(obj) {
				switch obj.(type) {
				case *v1.OperatorConfig:
					continue
				}
				localResources = append(localResources, obj)
				continue
			}
			globalResources = append(globalResources, obj)
		}
	}
	return
}

// isMetaNamespaced returns resources that are namespaced but whose configurations are bound to a
// namespaced object or another meta-namespaced resource, and thus we would need separate versions
// for separate namespaces.
func isMetaNamespaced(obj client.Object) bool {
	switch obj.(type) {
	case *corev1.Namespace, *rbacv1.ClusterRole, *rbacv1.ClusterRoleBinding, *admissionregistrationv1.ValidatingWebhookConfiguration, *admissionregistrationv1.MutatingWebhookConfiguration:
		return true
	}
	return false
}
