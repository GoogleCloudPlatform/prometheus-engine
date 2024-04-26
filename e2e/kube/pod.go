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
	"fmt"
	"io"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodLogs returns the logs of the pod with the given name.
func PodLogs(ctx context.Context, restConfig *rest.Config, namespace, name, container string) (string, error) {
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "", err
	}
	req := clientSet.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
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

func waitForPodContainerReady(ctx context.Context, kubeClient client.Client, pod *corev1.Pod, container string) error {
	return waitForResourceReady(ctx, kubeClient, pod, func(pod *corev1.Pod) error {
		return isPodContainerReady(pod, container)
	})
}

func isPodContainerReady(pod *corev1.Pod, container string) error {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == container {
			if !status.Ready {
				key := client.ObjectKeyFromObject(pod)
				return fmt.Errorf("pod %s container %s not ready: %s", key, status.Name, containerStatePretty(&status.State))
			}
			return nil
		}
	}
	key := client.ObjectKeyFromObject(pod)
	return fmt.Errorf("no container named %s found in pod %s", container, key)
}

func containerStatePretty(state *corev1.ContainerState) string {
	if state.Waiting != nil {
		return fmt.Sprintf("waiting due to %s", state.Waiting.Reason)
	}
	if state.Terminated != nil {
		return fmt.Sprintf("terminated due to %s", state.Terminated.Reason)
	}
	return "running"
}

func podByIP(ctx context.Context, kubeClient client.Client, ip net.IP) (*corev1.Pod, error) {
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

func podByAddr(ctx context.Context, kubeClient client.Client, addr *net.TCPAddr) (*corev1.Pod, string, error) {
	pod, err := podByIP(ctx, kubeClient, addr.IP)
	if err != nil {
		return nil, "", err
	}
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if int(port.ContainerPort) == addr.Port {
				return pod, container.Name, nil
			}
		}
	}
	key := client.ObjectKeyFromObject(pod)
	return nil, "", fmt.Errorf("unable to find port %d in pod %s", addr.Port, key)
}
