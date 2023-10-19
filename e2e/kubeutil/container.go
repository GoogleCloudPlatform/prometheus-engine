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
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func containerPods(ctx context.Context, kubeClient client.Client, o client.Object) ([]corev1.Pod, error) {
	switch o := o.(type) {
	case *appsv1.Deployment:
		return DeploymentPods(ctx, kubeClient, o)
	case *appsv1.DaemonSet:
		return DaemonSetPods(ctx, kubeClient, o)
	default:
		return nil, errors.New("invalid object type")
	}
}

func containerDebug(t testing.TB, ctx context.Context, restConfig *rest.Config, kubeClient client.Client, gvk schema.GroupVersionKind, o client.Object, typeName string) {
	namespace := o.GetNamespace()
	name := o.GetName()
	t.Logf("%s %s/%s events:", typeName, namespace, name)
	events, err := Events(ctx, kubeClient, gvk, namespace, name)
	if err != nil {
		t.Errorf("unable to get %s %s/%s events: %s", typeName, namespace, name, err)
	} else {
		t.Log(strings.Join(events, "\n"))
	}

	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(o), o); err != nil {
		t.Fatalf("unable to get deployment %s/%s: %s", namespace, name, err)
	}

	// Best effort to find a problematic pod container.
	pods, err := containerPods(ctx, kubeClient, o)
	if err != nil {
		t.Errorf("unable to get container pods")
	}
	if len(pods) == 0 {
		t.Log("no pods found for this deployment")
		return
	}

	showPodLogs := func(pod *corev1.Pod, container string) {
		t.Logf("sample pod %s/%s container %s logs:", pod.Namespace, pod.Name, container)
		logs, err := PodLogs(ctx, restConfig, pod.Namespace, pod.Name, container)
		if err != nil {
			t.Errorf("unable to get pod %s/%s container %s logs: %s", pod.Namespace, pod.Name, container, err)
		} else {
			t.Log(logs)
		}
	}

	for _, pod := range pods {
		found := false
		for _, status := range pod.Status.ContainerStatuses {
			// The pod is crash-looping.
			if status.RestartCount > 1 {
				found = true
				showPodLogs(&pod, status.Name)
			}
		}
		// Not perfect, but hopefully we found an issue.
		if found {
			return
		}
	}

	// Worse case, let's just show the first one.
	t.Log("found no crash-looping pods -- showing logs of first pod")
	for _, pod := range pods {
		for _, status := range pod.Status.ContainerStatuses {
			showPodLogs(&pod, status.Name)
		}
	}
}
