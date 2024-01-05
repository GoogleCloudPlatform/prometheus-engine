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

func IsPodMonitoringReady(pm PodMonitoringCRDWithScrapeEndpoints, targetStatusEnabled bool) error {
	for _, condition := range pm.GetStatus().Conditions {
		if condition.Type == monitoringv1.ConfigurationCreateSuccess {
			if condition.Status != corev1.ConditionTrue {
				return fmt.Errorf("configuration was not created successfully: %s", condition.Status)
			}
		} else {
			return fmt.Errorf("unknown condition type: %s", condition.Type)
		}
	}
	if !targetStatusEnabled {
		return nil
	}
	return isPodMonitoringEndpointStatusReady(pm)
}

type PodMonitoringCRDWithScrapeEndpoints interface {
	monitoringv1.PodMonitoringCRD

	// GetEndpoints returns the endpoints scraped by this CRD.
	GetEndpoints() []monitoringv1.ScrapeEndpoint
}

func isPodMonitoringEndpointStatusReady(pm PodMonitoringCRDWithScrapeEndpoints) error {
	endpointStatuses := pm.GetEndpointStatus()
	expectedEndpoints := len(pm.GetEndpoints())
	if size := len(endpointStatuses); size == 0 {
		return errors.New("empty endpoint status")
	} else if size != expectedEndpoints {
		return fmt.Errorf("expected %d endpoints, but got: %d", expectedEndpoints, size)
	}
	return nil
}

func WaitForPodMonitoringReady(ctx context.Context, kubeClient client.Client, operatorNamespace string, pm PodMonitoringCRDWithScrapeEndpoints, targetStatusEnabled bool) error {
	if err := WaitForCollectionReady(ctx, kubeClient, operatorNamespace); err != nil {
		return err
	}

	timeout := 2 * time.Minute
	interval := 3 * time.Second
	if targetStatusEnabled {
		// Wait for target status to get polled and populated.
		timeout = 4 * time.Minute
	}

	var err error
	pollErr := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		if err = kubeClient.Get(ctx, client.ObjectKeyFromObject(pm), pm); err != nil {
			return false, nil
		}

		if err = IsPodMonitoringReady(pm, targetStatusEnabled); err != nil {
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

func isPodMonitoringScrapeEndpointSuccess(status *monitoringv1.ScrapeEndpointStatus) error {
	var errs []error
	if status.UnhealthyTargets != 0 {
		errs = append(errs, fmt.Errorf("unhealthy targets: %d", status.UnhealthyTargets))
	}
	if status.CollectorsFraction != "1" {
		errs = append(errs, fmt.Errorf("collectors failed: %s", status.CollectorsFraction))
	}
	if len(status.SampleGroups) == 0 {
		errs = append(errs, errors.New("missing sample groups"))
	} else {
		for i, group := range status.SampleGroups {
			if len(group.SampleTargets) == 0 {
				errs = append(errs, fmt.Errorf("missing sample targets for group %d", i))
			} else {
				for _, target := range group.SampleTargets {
					if target.Health != "up" {
						lastErr := "no error reported"
						if target.LastError != nil {
							lastErr = *target.LastError
						}
						errs = append(errs, fmt.Errorf("unhealthy target %q at group %d: %s", target.Health, i, lastErr))
						break
					}
				}
			}
		}
	}
	return errors.Join(errs...)
}

func isPodMonitoringScrapeEndpointFailure(status *monitoringv1.ScrapeEndpointStatus, expectedFn func(message string) error) error {
	var errs []error
	if status.UnhealthyTargets == 0 {
		errs = append(errs, errors.New("expected no healthy targets"))
	}
	if status.CollectorsFraction == "0" {
		errs = append(errs, fmt.Errorf("expected collectors fraction to be 0 but found: %s", status.CollectorsFraction))
	}
	if len(status.SampleGroups) == 0 {
		errs = append(errs, errors.New("missing sample groups"))
	}
	for i, group := range status.SampleGroups {
		if len(group.SampleTargets) == 0 {
			errs = append(errs, fmt.Errorf("missing sample targets for group %d", i))
		}
		for _, target := range group.SampleTargets {
			if target.Health == "up" {
				errs = append(errs, fmt.Errorf("healthy target %q at group %d", target.Health, i))
				break
			}
			if target.LastError == nil {
				errs = append(errs, fmt.Errorf("missing error for target at group %d", i))
				break
			}
			if err := expectedFn(*target.LastError); err != nil {
				errs = append(errs, fmt.Errorf("for error message %q at group %d: got %w", *target.LastError, i, err))
				break
			}
		}
	}
	return errors.Join(errs...)
}

func IsPodMonitoringSuccess(pm PodMonitoringCRDWithScrapeEndpoints, targetStatusEnabled bool) error {
	if err := IsPodMonitoringReady(pm, targetStatusEnabled); err != nil {
		return err
	}
	if !targetStatusEnabled {
		return nil
	}
	var errs []error
	for _, status := range pm.GetEndpointStatus() {
		if err := isPodMonitoringScrapeEndpointSuccess(&status); err != nil {
			errs = append(errs, fmt.Errorf("unhealthy endpoint status %q: %w", status.Name, err))
		}
	}
	return errors.Join(errs...)
}

func WaitForPodMonitoringSuccess(ctx context.Context, kubeClient client.Client, pm PodMonitoringCRDWithScrapeEndpoints) error {
	var err error
	if pollErr := wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err = kubeClient.Get(ctx, client.ObjectKeyFromObject(pm), pm); err != nil {
			return false, nil
		}
		err = IsPodMonitoringSuccess(pm, true)
		return err == nil, nil
	}); pollErr != nil {
		if errors.Is(pollErr, context.DeadlineExceeded) && err != nil {
			return err
		}
		return pollErr
	}
	return nil
}

func IsPodMonitoringFailure(pm PodMonitoringCRDWithScrapeEndpoints, expectedFn func(message string) error) error {
	if err := IsPodMonitoringReady(pm, expectedFn != nil); err != nil {
		return err
	}
	var errs []error
	for _, status := range pm.GetEndpointStatus() {
		if err := isPodMonitoringScrapeEndpointFailure(&status, expectedFn); err != nil {
			errs = append(errs, fmt.Errorf("unhealthy endpoint status %q: %w", status.Name, err))
		}
	}
	return errors.Join(errs...)
}

func WaitForPodMonitoringFailure(ctx context.Context, kubeClient client.Client, pm PodMonitoringCRDWithScrapeEndpoints, expectedFn func(message string) error) error {
	var err error
	if pollErr := wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err = kubeClient.Get(ctx, client.ObjectKeyFromObject(pm), pm); err != nil {
			return false, nil
		}
		err = IsPodMonitoringFailure(pm, expectedFn)
		return err == nil, nil
	}); pollErr != nil {
		if errors.Is(pollErr, context.DeadlineExceeded) && err != nil {
			return err
		}
		return pollErr
	}
	return nil
}
