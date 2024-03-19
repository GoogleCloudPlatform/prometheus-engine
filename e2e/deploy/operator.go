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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitForOperatorReady(ctx context.Context, t testing.TB, kubeClient client.Client) error {
	dryRun := client.NewDryRunClient(kubeClient)
	pm := monitoringv1.PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "collector-podmon",
			Namespace: operator.DefaultOperatorNamespace,
		},
		Spec: monitoringv1.PodMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					operator.LabelAppName: operator.NameCollector,
				},
			},
			Endpoints: []monitoringv1.ScrapeEndpoint{
				{
					Port:     intstr.FromString(operator.CollectorPrometheusContainerPortName),
					Interval: "5s",
				},
			},
		},
	}
	return wait.PollUntilContextCancel(ctx, 3*time.Second, true, func(ctx context.Context) (bool, error) {
		if err := dryRun.Create(ctx, &pm); err != nil {
			// Expected to have a forbidden PodMonitoring until we're ready.
			if !apierrors.IsForbidden(err) {
				t.Logf("unable to create PodMonitoring: %s", err)
			}
			return false, nil
		}
		return true, nil
	})
}

// OperatorLogs returns the operator pods logs.
func OperatorLogs(ctx context.Context, kubeClient client.Client, clientSet kubernetes.Interface, operatorNamespace string) (string, error) {
	pod, err := operatorPod(ctx, kubeClient, operatorNamespace)
	if err != nil {
		return "", err
	}
	return kube.PodLogs(ctx, clientSet, pod.Namespace, pod.Name, "operator")
}

func operatorPod(ctx context.Context, kubeClient client.Client, operatorNamespace string) (*corev1.Pod, error) {
	podList, err := kube.DeploymentPods(ctx, kubeClient, operatorNamespace, operator.NameOperator)
	if err != nil {
		return nil, err
	}
	if len(podList.Items) != 1 {
		return nil, fmt.Errorf("expected 1 pod, found %d", len(podList.Items))
	}
	return &podList.Items[0], nil
}
