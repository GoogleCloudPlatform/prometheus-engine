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
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	GMPAPIVersion = "monitoring.googleapis.com/v1"

	KindPodMonitoring        = "PodMonitoring"
	KindClusterPodMonitoring = "ClusterPodMonitoring"
	KindOperatorConfig       = "OperatorConfig"
	KindPodMonitor           = "PodMonitor"
	KindServiceMonitor       = "ServiceMonitor"
	KindPrometheus           = "Prometheus"
	KindService              = "Service"
	KindConfigMap            = "ConfigMap"
	KindSecret               = "Secret"
)

// ResourceConverter defines the interface for converting a specific Prometheus Operator resource kind.
type ResourceConverter interface {
	// ImportKey returns the Kind of the resource this converter handles (e.g., "PodMonitor").
	ImportKey() string
	// Convert translates the input unstructured resource to one or more GMP resources.
	Convert(ctx context.Context, logger *slog.Logger, unstruct *unstructured.Unstructured, cache *ResourceCache) (outputs []*unstructured.Unstructured, err error)
}

// ResourceCache stores parsed Kubernetes resources for cross-resource resolution.
type ResourceCache struct {
	// Map of Kind -> Namespace/Name -> Resource.
	resources map[string]map[string]*unstructured.Unstructured
}

// NewResourceCache creates a new initialized ResourceCache.
func NewResourceCache() *ResourceCache {
	return &ResourceCache{
		resources: make(map[string]map[string]*unstructured.Unstructured),
	}
}

// Add adds a resource to the cache, returning an error if inputs are invalid.
func (c *ResourceCache) Add(u *unstructured.Unstructured) error {
	if c == nil {
		return errors.New("cannot add to nil ResourceCache")
	}
	if u == nil {
		return errors.New("cannot add nil resource to cache")
	}

	name := u.GetName()
	if name == "" {
		return errors.New("cannot add resource with empty name to cache")
	}
	kind := u.GetKind()
	if kind == "" {
		return errors.New("cannot add resource with empty kind to cache")
	}
	apiVersion := u.GetAPIVersion()
	if apiVersion == "" {
		return errors.New("cannot add resource with empty apiVersion to cache")
	}

	if c.resources == nil {
		c.resources = make(map[string]map[string]*unstructured.Unstructured)
	}

	if _, ok := c.resources[kind]; !ok {
		c.resources[kind] = make(map[string]*unstructured.Unstructured)
	}

	ns := u.GetNamespace()

	key := fmt.Sprintf("%s/%s", ns, name)
	if _, exists := c.resources[kind][key]; exists {
		return fmt.Errorf("duplicate resource %s/%s found in cache", kind, key)
	}
	c.resources[kind][key] = u
	return nil
}

// Get retrieves a resource from the cache by kind, namespace, and name.
func (c *ResourceCache) Get(kind, namespace, name string) (*unstructured.Unstructured, bool) {
	if c == nil || c.resources == nil {
		return nil, false
	}
	nsMap, ok := c.resources[kind]
	if !ok {
		return nil, false
	}
	key := fmt.Sprintf("%s/%s", namespace, name)
	r, ok := nsMap[key]
	return r, ok
}

// findServicesBySelector finds Services matching the label selector within the specified namespaces.
// Note for callers: passing nil or an empty namespaces slice matches Services across every namespace.
// Callers should use determineNamespaceScoping to resolve default namespace rules before calling this method.
func (c *ResourceCache) findServicesBySelector(selector metav1.LabelSelector, namespaces []string) ([]*unstructured.Unstructured, error) {
	if c == nil || c.resources == nil {
		return nil, nil
	}

	sel, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, fmt.Errorf("invalid selector: %w", err)
	}

	var matched []*unstructured.Unstructured

	services, ok := c.resources[KindService]
	if !ok {
		return nil, nil
	}

	// To make output deterministic, we sort the keys before iterating.
	keys := slices.AppendSeq(make([]string, 0, len(services)), maps.Keys(services))
	slices.Sort(keys)

	for _, key := range keys {
		svc := services[key]
		svcNS := svc.GetNamespace()

		if len(namespaces) > 0 && !slices.Contains(namespaces, svcNS) {
			continue
		}

		svcLabels := svc.GetLabels()
		if sel.Matches(labels.Set(svcLabels)) {
			matched = append(matched, svc)
		}
	}
	return matched, nil
}
