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

package kubeutil

import (
	"context"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodLogs returns the logs of the pod with the given name.
func PodLogs(ctx context.Context, restConfig *rest.Config, namespace, name, container string) (string, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "", err
	}
	req := clientset.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		Container: container,
	})
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	builder := strings.Builder{}
	if _, err := io.Copy(&builder, podLogs); err != nil {
		return "", err
	}
	return builder.String(), nil
}

// Events returns the events of the given resource.
func Events(ctx context.Context, kubeClient client.Client, gvk schema.GroupVersionKind, namespace, name string) ([]string, error) {
	var msgs []string
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
	for _, ev := range events.Items {
		msgs = append(msgs, ev.Message)
	}
	return msgs, nil
}
