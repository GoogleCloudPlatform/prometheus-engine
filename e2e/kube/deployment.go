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

package kube

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DeploymentContainer(deployment *appsv1.Deployment, name string) (*corev1.Container, error) {
	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]
		if container.Name == name {
			return container, nil
		}
	}
	return nil, fmt.Errorf("unable to find container %q", name)
}

// Copied from https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/deployment/util/deployment_util.go
// DeploymentComplete considers a deployment to be complete once all of its desired replicas
// are updated and available, and no old pods are running.
func DeploymentComplete(deployment *appsv1.Deployment, newStatus *appsv1.DeploymentStatus) bool {
	return newStatus.UpdatedReplicas == *(deployment.Spec.Replicas) &&
		newStatus.Replicas == *(deployment.Spec.Replicas) &&
		newStatus.AvailableReplicas == *(deployment.Spec.Replicas) &&
		newStatus.ObservedGeneration >= deployment.Generation
}

func WaitForDeploymentReady(ctx context.Context, kubeClient client.Client, namespace, name string) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return waitForResourceReady(ctx, kubeClient, deployment, func(deployment *appsv1.Deployment) error {
		if DeploymentComplete(deployment, &deployment.Status) {
			return nil
		}
		return errors.New("replicas updating")
	})
}
