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

	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WaitForOperatorReady waits until the GMP operator is ready to serve webhooks.
func WaitForOperatorReady(ctx context.Context, kubeClient client.Client) error {
	if err := kube.WaitForDeploymentReady(ctx, kubeClient, operator.DefaultOperatorNamespace, operator.NameOperator); err != nil {
		return err
	}
	webhookName := fmt.Sprintf("%s.%s.monitoring.googleapis.com", operator.NameOperator, operator.DefaultOperatorNamespace)
	return wait.PollUntilContextCancel(ctx, 500*time.Millisecond, true, func(ctx context.Context) (bool, error) {
		var vwc admissionregistrationv1.ValidatingWebhookConfiguration
		if err := kubeClient.Get(ctx, client.ObjectKey{Name: webhookName}, &vwc); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if len(vwc.Webhooks) == 0 {
			return false, nil
		}
		for _, w := range vwc.Webhooks {
			if len(w.ClientConfig.CABundle) == 0 {
				return false, nil
			}
		}
		return true, nil
	})
}

// OperatorLogs returns the operator pods logs.
func OperatorLogs(ctx context.Context, restConfig *rest.Config, kubeClient client.Client, operatorNamespace string) (string, error) {
	pod, err := operatorPod(ctx, kubeClient, operatorNamespace)
	if err != nil {
		return "", err
	}
	return kube.PodLogs(ctx, restConfig, pod.Namespace, pod.Name, "operator")
}

func operatorPod(ctx context.Context, kubeClient client.Client, operatorNamespace string) (*corev1.Pod, error) {
	podList, err := kube.DeploymentPods(ctx, kubeClient, operatorNamespace, operator.NameOperator)
	if err != nil {
		return nil, err
	}
	podList = kube.PodsReady(podList)
	if len(podList) != 1 {
		return nil, fmt.Errorf("expected 1 pod, found %d", len(podList))
	}
	return &podList[0], nil
}
