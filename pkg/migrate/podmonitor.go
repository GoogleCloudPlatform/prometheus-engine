// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package migrate

import (
	"context"
	"fmt"
	"log/slog"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// PodMonitorConverter implements ResourceConverter for PodMonitor resources.
type PodMonitorConverter struct{}

// ImportKey returns the Kind of the resource this converter handles.
func (c *PodMonitorConverter) ImportKey() string {
	return "PodMonitor"
}

// Convert translates a Prometheus Operator PodMonitor into GMP resources.
func (c *PodMonitorConverter) Convert(ctx context.Context, logger *slog.Logger, unstruct *unstructured.Unstructured, cache *ResourceCache) ([]*unstructured.Unstructured, error) {
	if unstruct == nil {
		return nil, fmt.Errorf("cannot convert nil unstructured resource")
	}

	// 1. Unmarshal unstructured to typed PodMonitor
	var podMonitor monitoringv1.PodMonitor
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &podMonitor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PodMonitor: %w", err)
	}

	logger.Info("Successfully decoded PodMonitor", slog.String("name", podMonitor.Name))

	// TODO: Implement actual conversion logic in subsequent steps

	return nil, nil
}
