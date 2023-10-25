// Copyright 2023 Google LLC
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
package operatorutil

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitForOperatorReady(ctx context.Context, kubeClient client.Client, operatorNamespace string) error {
	// Assume that the existence of the config means that the reconcile loop started.
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		var config corev1.ConfigMap
		if err := kubeClient.Get(ctx, client.ObjectKey{Name: operator.NameCollector, Namespace: operatorNamespace}, &config); err != nil {
			return false, nil
		}
		return true, nil
	})
}
