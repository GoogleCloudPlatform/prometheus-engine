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
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const validatingWebhookName = "gmp-operator.gmp-system.monitoring.googleapis.com"

func WaitForOperatorReady(ctx context.Context, kubeClient client.Client, operatorNamespace string) error {
	// The existence of the webhook CA cert means that the operator reconcile loop started.
	return wait.PollUntilContextCancel(ctx, 3*time.Second, true, func(ctx context.Context) (bool, error) {
		config := admissionregistrationv1.ValidatingWebhookConfiguration{}
		if err := kubeClient.Get(ctx, client.ObjectKey{Name: validatingWebhookName, Namespace: operatorNamespace}, &config); err != nil {
			//nolint:nilerr // return nil to continue polling.
			return false, nil
		}
		return len(config.Webhooks) > 0 && len(config.Webhooks[0].ClientConfig.CABundle) > 0, nil
	})
}
