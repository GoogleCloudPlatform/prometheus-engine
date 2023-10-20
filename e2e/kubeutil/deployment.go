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

// Package e2e contains tests that validate the behavior of gmp-operator against a cluster.
// To make tests simple and fast, the test suite runs the operator internally. The CRDs
// are expected to be installed out of band (along with the operator deployment itself in
// a real world setup).
package kubeutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsDeploymentReady returns nil if the Deployment obtained from the given namespace and name
// has all expected replicas ready, otherwise returns an error why the Deployment is not ready.
func IsDeploymentReady(ctx context.Context, kubeClient client.Client, namespace, name string) error {
	wrapErrFunc := func(err error) error {
		return fmt.Errorf("deployment %s/%s not ready: %w", namespace, name, err)
	}
	var deployment appsv1.Deployment
	if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &deployment); err != nil {
		return wrapErrFunc(fmt.Errorf("not found: %w", err))
	}

	// Set to default replicas value.
	expected := int32(1)
	if deployment.Spec.Replicas != nil {
		expected = *deployment.Spec.Replicas
	}
	if deployment.Status.ReadyReplicas != expected {
		return wrapErrFunc(errors.New("replicas unavailable"))
	}
	return nil
}

func WaitForDeploymentReady(ctx context.Context, kubeClient client.Client, namespace, name string) error {
	var err error
	if waitErr := wait.Poll(3*time.Second, 1*time.Minute, func() (bool, error) {
		err := IsDeploymentReady(ctx, kubeClient, namespace, name)
		return err == nil, nil
	}); waitErr != nil {
		if errors.Is(waitErr, wait.ErrWaitTimeout) {
			return err
		}
		return waitErr
	}
	return nil
}

func DeploymentContainer(deployment *appsv1.Deployment, name string) (*corev1.Container, error) {
	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]
		if container.Name == name {
			return container, nil
		}
	}
	return nil, fmt.Errorf("unable to find container %q", name)
}
