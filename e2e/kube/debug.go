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
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// Events returns the events of the given resource.
func Events(ctx context.Context, kubeClient client.Client, gvk schema.GroupVersionKind, namespace, name string) ([]string, error) {
	events := corev1.EventList{}
	apiVersion, kind := gvk.ToAPIVersionAndKind()
	if err := kubeClient.List(ctx, &events, client.InNamespace(namespace), client.MatchingFieldsSelector{
		Selector: fields.SelectorFromSet(fields.Set(map[string]string{
			"involvedObject.apiVersion": apiVersion,
			"involvedObject.kind":       kind,
			"involvedObject.name":       name,
		})),
	}); err != nil {
		return nil, err
	}
	msgs := make([]string, 0, len(events.Items))
	for _, ev := range events.Items {
		msgs = append(msgs, ev.Message)
	}
	return msgs, nil
}

// Debug prints both events and logs of the given resource. Consider passing a new context
// here in case the original context is cancelled.
func Debug(ctx context.Context, restConfig *rest.Config, kubeClient client.Client, o client.Object, out io.Writer) error {
	gvk, err := apiutil.GVKForObject(o, kubeClient.Scheme())
	if err != nil {
		return fmt.Errorf("unable to get GVK")
	}

	namespace := o.GetNamespace()
	name := o.GetName()
	typeName := gvk.String()
	events, err := Events(ctx, kubeClient, gvk, namespace, name)
	if err != nil {
		return fmt.Errorf("unable to get %s %s/%s events: %w", typeName, namespace, name, err)
	}
	fmt.Fprintf(out, "= %s %s/%s Debug\n", typeName, namespace, name)
	fmt.Fprint(out, "== Events:\n")
	fmt.Fprint(out, strings.Join(events, "\n"))
	fmt.Fprint(out, "\n")

	pods, err := workloadPods(ctx, kubeClient, o)
	if err != nil {
		return fmt.Errorf("unable to get %s %s/%s pods: %w", typeName, namespace, name, err)
	}
	if len(pods) == 0 {
		fmt.Fprint(out, "No pods found.\n")
		return nil
	}

	showPodLogs := func(pod *corev1.Pod, container string) error {
		const amount = 20
		logs, err := PodLogs(ctx, restConfig, pod.Namespace, pod.Name, container)
		if err != nil {
			return fmt.Errorf("unable to get container %s logs: %w", container, err)
		}
		fmt.Fprintf(out, "== Logs (Container %q) (last %d lines):\n", container, amount)
		lines := strings.Split(logs, "\n")
		if len(lines) > amount {
			lines = lines[len(lines)-amount:]
		}
		fmt.Fprint(out, strings.Join(lines, "\n"))
		return nil
	}

	// Best effort to find the problematic Pod container.
	for _, pod := range pods {
		found := false
		for _, status := range pod.Status.ContainerStatuses {
			// The Pod is crash-looping: definite issue.
			if status.RestartCount > 1 {
				found = true
				if err := showPodLogs(&pod, status.Name); err != nil {
					return err
				}
			}
		}
		// We might not have found the real problem, but a problem for sure.
		if found {
			return nil
		}
	}

	if len(pods) == 0 {
		return nil
	}

	// Worse case, let's just show the first one.
	pod := pods[0]
	for _, status := range pod.Status.ContainerStatuses {
		if err := showPodLogs(&pod, status.Name); err != nil {
			return err
		}
	}
	return nil
}

func workloadPods(ctx context.Context, kubeClient client.Client, o client.Object) ([]corev1.Pod, error) {
	switch o := o.(type) {
	case *corev1.Pod:
		return []corev1.Pod{*o}, nil
	case *appsv1.Deployment:
		return DeploymentPods(ctx, kubeClient, o.Namespace, o.Name)
	default:
		return nil, errors.New("invalid object type")
	}
}
