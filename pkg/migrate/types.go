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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
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

const (
	indexKind          = "kind"
	indexKindNamespace = "kindNamespace"
)

// CachedResource holds the raw unstructured object and optional pre-converted typed struct.
type CachedResource struct {
	Unstructured *unstructured.Unstructured
	TypedService *corev1.Service
}

// compareObjectMeta compares two Kubernetes resources deterministically by Namespace first, then by Name.
func compareObjectMeta[T metav1.Object](a, b T) int {
	if a.GetNamespace() != b.GetNamespace() {
		return strings.Compare(a.GetNamespace(), b.GetNamespace())
	}
	return strings.Compare(a.GetName(), b.GetName())
}

// ResourceCache stores parsed Kubernetes resources for cross-resource resolution.
type ResourceCache struct {
	indexer cache.Indexer
}

func getResourceKey(kind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}

// NewResourceCache creates a new initialized ResourceCache.
func NewResourceCache() *ResourceCache {
	indexers := cache.Indexers{
		indexKind: func(obj any) ([]string, error) {
			res, ok := obj.(*CachedResource)
			if !ok {
				return nil, fmt.Errorf("expected *CachedResource, got %T", obj)
			}
			return []string{res.Unstructured.GetKind()}, nil
		},
		indexKindNamespace: func(obj any) ([]string, error) {
			res, ok := obj.(*CachedResource)
			if !ok {
				return nil, fmt.Errorf("expected *CachedResource, got %T", obj)
			}
			return []string{fmt.Sprintf("%s/%s", res.Unstructured.GetKind(), res.Unstructured.GetNamespace())}, nil
		},
	}

	keyFunc := func(obj any) (string, error) {
		res, ok := obj.(*CachedResource)
		if !ok {
			return "", fmt.Errorf("expected *CachedResource, got %T", obj)
		}
		return getResourceKey(res.Unstructured.GetKind(), res.Unstructured.GetNamespace(), res.Unstructured.GetName()), nil
	}

	return &ResourceCache{
		indexer: cache.NewIndexer(keyFunc, indexers),
	}
}

// Add adds a resource to the cache, returning an error if inputs are invalid.
func (c *ResourceCache) Add(u *unstructured.Unstructured) error {
	if c == nil || c.indexer == nil {
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

	ns := u.GetNamespace()
	key := getResourceKey(kind, ns, name)
	if _, exists, _ := c.indexer.GetByKey(key); exists {
		return fmt.Errorf("duplicate resource %s/%s found in cache", kind, key)
	}

	res := &CachedResource{
		Unstructured: u,
	}

	if kind == KindService {
		var svc corev1.Service
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &svc); err != nil {
			return fmt.Errorf("failed to convert Service %s/%s to corev1.Service: %w", ns, name, err)
		}
		res.TypedService = &svc
	}

	return c.indexer.Add(res)
}

// Get retrieves a resource from the cache by kind, namespace, and name.
func (c *ResourceCache) Get(kind, namespace, name string) (*unstructured.Unstructured, bool) {
	if c == nil || c.indexer == nil {
		return nil, false
	}
	key := getResourceKey(kind, namespace, name)
	item, exists, err := c.indexer.GetByKey(key)
	if err != nil || !exists {
		return nil, false
	}
	res, ok := item.(*CachedResource)
	if !ok || res.Unstructured == nil {
		return nil, false
	}
	return res.Unstructured, true
}

// ListKinds returns a sorted slice of all resource kinds currently in the cache.
func (c *ResourceCache) ListKinds() []string {
	if c == nil || c.indexer == nil {
		return nil
	}
	kinds := c.indexer.ListIndexFuncValues(indexKind)
	slices.Sort(kinds)
	return kinds
}

// ListByKind returns a sorted slice of unstructured resources for a specified kind.
func (c *ResourceCache) ListByKind(kind string) []*unstructured.Unstructured {
	if c == nil || c.indexer == nil {
		return nil
	}
	items, err := c.indexer.ByIndex(indexKind, kind)
	if err != nil || len(items) == 0 {
		return nil
	}
	var res []*unstructured.Unstructured
	for _, item := range items {
		if r, ok := item.(*CachedResource); ok && r.Unstructured != nil {
			res = append(res, r.Unstructured)
		}
	}
	slices.SortFunc(res, compareObjectMeta)
	return res
}

// findServicesBySelector finds Services matching the label selector within the specified namespaces.
// Note for callers: passing nil or an empty namespaces slice matches Services across every namespace.
// Callers should use determineNamespaceScoping to resolve default namespace rules before calling this method.
func (c *ResourceCache) findServicesBySelector(selector metav1.LabelSelector, namespaces []string) ([]*corev1.Service, error) {
	if c == nil || c.indexer == nil {
		return nil, nil
	}

	sel, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, fmt.Errorf("invalid selector: %w", err)
	}

	var candidateItems []any
	if len(namespaces) > 0 {
		for _, ns := range namespaces {
			items, err := c.indexer.ByIndex(indexKindNamespace, fmt.Sprintf("%s/%s", KindService, ns))
			if err != nil {
				return nil, err
			}
			candidateItems = append(candidateItems, items...)
		}
	} else {
		var err error
		candidateItems, err = c.indexer.ByIndex(indexKind, KindService)
		if err != nil {
			return nil, err
		}
	}

	var matched []*corev1.Service
	for _, item := range candidateItems {
		res, ok := item.(*CachedResource)
		if !ok || res.TypedService == nil {
			continue
		}
		svc := res.TypedService
		if sel.Matches(labels.Set(svc.Labels)) {
			matched = append(matched, svc)
		}
	}

	slices.SortFunc(matched, compareObjectMeta)

	return matched, nil
}
