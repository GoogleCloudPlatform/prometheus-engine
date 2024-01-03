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

package operatorutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func isProbeStatusReady(pm *monitoringv1.Probe) error {
	endpointStatuses := pm.Status.EndpointStatuses
	expectedEndpoints := len(pm.Spec.Targets)
	if size := len(endpointStatuses); size == 0 {
		return errors.New("empty endpoint status")
	} else if size != expectedEndpoints {
		return fmt.Errorf("expected %d endpoints, but got: %d", expectedEndpoints, size)
	}
	return nil
}

func isProbeReady(pm *monitoringv1.Probe) error {
	for _, condition := range pm.Status.Conditions {
		if condition.Type == monitoringv1.ConfigurationCreateSuccess {
			if condition.Status != corev1.ConditionTrue {
				return fmt.Errorf("configuration was not created successfully: %s", condition.Status)
			}
		} else {
			return fmt.Errorf("unknown condition type: %s", condition.Type)
		}
	}
	return isProbeStatusReady(pm)
}

func WaitForProbeReady(ctx context.Context, kubeClient client.Client, operatorNamespace string, pm *monitoringv1.Probe) error {
	if err := WaitForCollectionReady(ctx, kubeClient, operatorNamespace); err != nil {
		return err
	}

	var err error
	pollErr := wait.PollUntilContextTimeout(ctx, time.Second*3, time.Minute*3, true, func(ctx context.Context) (bool, error) {
		if err = kubeClient.Get(ctx, client.ObjectKeyFromObject(pm), pm); err != nil {
			return false, nil
		}

		if err = isProbeReady(pm); err != nil {
			return false, nil
		}
		return true, nil
	})
	if pollErr != nil {
		if errors.Is(pollErr, context.DeadlineExceeded) && err != nil {
			return err
		}
		return pollErr
	}
	return nil
}
