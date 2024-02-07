// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package deploy

import (
	"errors"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/examples/instrumentation"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	SyntheticAppPortName      = "web"
	SyntheticAppContainerName = "go-synthetic"
)

func SyntheticAppResources(scheme *runtime.Scheme) (*appsv1.Deployment, *corev1.Service, error) {
	resources, err := kube.ResourcesFromYAML(scheme, instrumentation.SyntheticManifest)
	if err != nil {
		return nil, nil, err
	}

	var deployment *appsv1.Deployment
	var service *corev1.Service
	for _, resource := range resources {
		switch obj := resource.(type) {
		case *appsv1.Deployment:
			if obj.Name != SyntheticAppContainerName {
				continue
			}
			deployment = obj
		case *corev1.Service:
			if obj.Name != SyntheticAppContainerName {
				continue
			}
			service = obj
		}
	}
	if deployment == nil {
		return nil, nil, errors.New("unable to find deployment")
	}
	if service == nil {
		return nil, nil, errors.New("unable to find service")
	}
	return deployment, service, nil
}
