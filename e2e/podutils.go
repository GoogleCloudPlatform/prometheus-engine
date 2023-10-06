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
package e2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func containerStateString(state *corev1.ContainerState) string {
	if state.Waiting != nil {
		return fmt.Sprintf("waiting due to %s", state.Waiting.Reason)
	}
	if state.Terminated != nil {
		return fmt.Sprintf("terminated due to %s", state.Terminated.Reason)
	}
	return "running"
}

func isPodReady(ctx context.Context, restConfig *rest.Config, pod *corev1.Pod) error {
	var errs []error
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			key := client.ObjectKeyFromObject(pod)
			errs = append(errs, fmt.Errorf("pod %s container %s not ready: %s", key, status.Name, containerStateString(&status.State)))
		}
	}
	return errors.Join(errs...)
}

func waitUntilPodReady(ctx context.Context, t *testing.T, restConfig *rest.Config, kubeClient client.Client, pod *corev1.Pod) error {
	// Prevent doing an extra API lookup by checking first.
	var err error
	if err = isPodReady(ctx, restConfig, pod); err == nil {
		return nil
	}
	t.Logf("waiting for pod to be ready: %s", err)
	if waitErr := wait.Poll(2*time.Second, 30*time.Second, func() (done bool, err error) {
		if err = kubeClient.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
			return false, err
		}
		err = isPodReady(ctx, restConfig, pod)
		return err == nil, nil
	}); waitErr != nil {
		if errors.Is(waitErr, wait.ErrWaitTimeout) {
			return err
		}
		return waitErr
	}
	return nil
}

func getPodByIP(ctx context.Context, kubeClient client.Client, ip net.IP) (*corev1.Pod, error) {
	var pods corev1.PodList
	if err := kubeClient.List(ctx, &pods, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("status.podIP", ip.String()),
	}); err != nil {
		return nil, err
	}
	if len(pods.Items) != 1 {
		return nil, fmt.Errorf("expected 1 pod with IP %s, got %d", ip.String(), len(pods.Items))
	}
	return &pods.Items[0], nil
}
