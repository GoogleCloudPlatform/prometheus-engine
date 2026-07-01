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
	"errors"
	"fmt"
	"log/slog"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	pomonitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// PodMonitorConverter implements ResourceConverter for PodMonitor resources.
type PodMonitorConverter struct{}

// ImportKey returns the Kind of the resource this converter handles.
func (c *PodMonitorConverter) ImportKey() string {
	return KindPodMonitor
}

// Convert translates a Prometheus Operator PodMonitor into GMP resources.
func (c *PodMonitorConverter) Convert(_ context.Context, logger *slog.Logger, unstruct *unstructured.Unstructured, _ *ResourceCache) ([]*unstructured.Unstructured, error) {
	if unstruct == nil || unstruct.Object == nil {
		return nil, errors.New("cannot convert nil or uninitialized unstructured resource")
	}

	// 1. Unmarshal unstructured to typed PodMonitor
	var podMonitor pomonitoringv1.PodMonitor
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &podMonitor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PodMonitor: %w", err)
	}

	logger.Info("Successfully decoded PodMonitor", slog.String("name", podMonitor.Name))

	// 2. Determine Scoping based on namespaceSelector
	nsSel := podMonitor.Spec.NamespaceSelector

	if nsSel.Any {
		// Case A: namespaceSelector.any = true -> Single ClusterPodMonitoring
		logger.Info("namespaceSelector selects 'any: true'. Translated to 'ClusterPodMonitoring'")
		u, err := c.convertToClusterPodMonitoring(&podMonitor)
		if err != nil {
			return nil, err
		}
		return []*unstructured.Unstructured{u}, nil
	}

	if len(nsSel.MatchNames) > 0 {
		// Case B: namespaceSelector.matchNames listed -> Multiple PodMonitoring resources (one per namespace)
		targetNamespaces := ParseAndCleanNamespaces(nsSel.MatchNames)

		// 2.1 Fail if all provided names were empty/whitespace (broken config)
		if len(targetNamespaces) == 0 {
			return nil, errors.New("namespaceSelector.matchNames contains only empty or invalid values")
		}

		if len(targetNamespaces) > 1 {
			logger.Info("namespaceSelector targets multiple namespaces. Generating separate PodMonitoring resources for each namespace",
				slog.Any("namespaces", targetNamespaces),
			)
		}

		// 2.2 Convert to a base namespaced PodMonitoring
		baseU, err := c.convertToPodMonitoring(&podMonitor)
		if err != nil {
			return nil, err
		}

		// 2.3 Clone and apply target namespaces
		var outputs []*unstructured.Unstructured
		for _, ns := range targetNamespaces {
			uClone := baseU.DeepCopy()
			uClone.SetNamespace(ns)
			outputs = append(outputs, uClone)
		}
		return outputs, nil
	}

	// Case C: namespaceSelector is empty/omitted -> Single PodMonitoring in local namespace
	u, err := c.convertToPodMonitoring(&podMonitor)
	if err != nil {
		return nil, err
	}
	return []*unstructured.Unstructured{u}, nil
}

func (c *PodMonitorConverter) convertToPodMonitoring(pm *pomonitoringv1.PodMonitor) (*unstructured.Unstructured, error) {
	gmpPM := &monitoringv1.PodMonitoring{
		TypeMeta:   BuildTypeMeta(KindPodMonitoring),
		ObjectMeta: CopyObjectMeta(pm.ObjectMeta, pm.Namespace),
		Spec: monitoringv1.PodMonitoringSpec{
			Selector: pm.Spec.Selector,
			// TODO: Migrate pm.Spec.PodMetricsEndpoints to Spec.Endpoints in subsequent step.
		},
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gmpPM)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PodMonitoring: %w", err)
	}

	// Explicitly restore type meta as ToUnstructured sometimes strips it
	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindPodMonitoring)

	return u, nil
}

func (c *PodMonitorConverter) convertToClusterPodMonitoring(pm *pomonitoringv1.PodMonitor) (*unstructured.Unstructured, error) {
	gmpCPM := &monitoringv1.ClusterPodMonitoring{
		TypeMeta:   BuildTypeMeta(KindClusterPodMonitoring),
		ObjectMeta: CopyObjectMeta(pm.ObjectMeta, ""), // Cluster-scoped, namespace is omitted
		Spec: monitoringv1.ClusterPodMonitoringSpec{
			Selector: pm.Spec.Selector,
			// TODO: Migrate pm.Spec.PodMetricsEndpoints to Spec.Endpoints in subsequent step.
		},
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gmpCPM)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ClusterPodMonitoring: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredMap}
	u.SetAPIVersion(GMPAPIVersion)
	u.SetKind(KindClusterPodMonitoring)

	return u, nil
}
