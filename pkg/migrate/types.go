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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceConverter defines the interface for converting a specific Kubernetes resource kind.
type ResourceConverter interface {
	// ImportKey returns the Kind of the resource this converter handles (e.g., "PodMonitor").
	ImportKey() string
	// Convert translates the input unstructured resource to one or more GKE resources.
	Convert(ctx context.Context, logger *slog.Logger, unstruct *unstructured.Unstructured, cache *ResourceCache) (outputs []*unstructured.Unstructured, err error)
}

// ResourceCache stores parsed Kubernetes resources for cross-resource resolution.
type ResourceCache struct {
	// Map of Kind -> Namespace/Name -> Resource
	resources map[string]map[string]*unstructured.Unstructured
}

// NewResourceCache creates a new initialized ResourceCache.
func NewResourceCache() *ResourceCache {
	return &ResourceCache{
		resources: make(map[string]map[string]*unstructured.Unstructured),
	}
}

// Add adds a resource to the cache, defaulting empty namespaces to "default".
func (c *ResourceCache) Add(u *unstructured.Unstructured) {
	kind := u.GetKind()
	if _, ok := c.resources[kind]; !ok {
		c.resources[kind] = make(map[string]*unstructured.Unstructured)
	}

	ns := u.GetNamespace()
	if ns == "" {
		ns = "default"
		u.SetNamespace("default") // Explicitly write "default" namespace to the object.
	}

	key := fmt.Sprintf("%s/%s", ns, u.GetName())
	c.resources[kind][key] = u
}

// Get retrieves a resource from the cache by kind, namespace, and name.
func (c *ResourceCache) Get(kind, namespace, name string) (*unstructured.Unstructured, bool) {
	nsMap, ok := c.resources[kind]
	if !ok {
		return nil, false
	}
	key := fmt.Sprintf("%s/%s", namespace, name)
	r, ok := nsMap[key]
	return r, ok
}
