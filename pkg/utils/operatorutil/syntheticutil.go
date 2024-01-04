// Copyright 2023 Google LLC
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

package operatorutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/utils/kubeutil"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SyntheticAppPortName      = "web"
	SyntheticAppContainerName = "go-synthetic"

	appManifest = "../examples/instrumentation/go-synthetic/go-synthetic.yaml"
)

func syntheticAppDeployment(scheme *runtime.Scheme) (*appsv1.Deployment, error) {
	resources, err := kubeutil.ResourcesFromFile(scheme, appManifest)
	if err != nil {
		return nil, err
	}

	var deployment *appsv1.Deployment
	for _, resource := range resources {
		var ok bool
		deployment, ok = resource.(*appsv1.Deployment)
		if ok {
			break
		}
	}
	if deployment == nil {
		return nil, errors.New("unable to find app deployment")
	}
	return deployment, nil
}

func SyntheticAppDeploy(ctx context.Context, kubeClient client.Client, namespace, name string, args []string) (*appsv1.Deployment, error) {
	deployment, err := syntheticAppDeployment(kubeClient.Scheme())
	if err != nil {
		return nil, err
	}

	deployment.Namespace = namespace
	deployment.Name = name
	if deployment.Spec.Template.Labels == nil {
		deployment.Spec.Template.Labels = map[string]string{}
	}
	deployment.Spec.Template.Labels[operator.LabelInstanceName] = name

	container, err := kubeutil.DeploymentContainer(deployment, SyntheticAppContainerName)
	if err != nil {
		return nil, err
	}
	container.Args = append(container.Args, args...)

	if err := kubeClient.Create(ctx, deployment); err != nil {
		return nil, fmt.Errorf("unable to create app deployment: %w", err)
	}
	return deployment, nil
}
