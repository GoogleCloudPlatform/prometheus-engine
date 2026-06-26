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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildObjectMeta constructs standard ObjectMeta with name and namespace.
func BuildObjectMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}

// BuildTypeMeta constructs standard TypeMeta for a GMP resource Kind.
func BuildTypeMeta(kind string) metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: GMPAPIVersion,
		Kind:       kind,
	}
}

// ParseAndCleanNamespaces trims whitespace, filters out empty strings, and deduplicates namespaces
func ParseAndCleanNamespaces(namespaces []string) []string {
	unique := make(map[string]bool)
	var cleaned []string
	for _, ns := range namespaces {
		trimmed := strings.TrimSpace(ns)
		if trimmed != "" && !unique[trimmed] {
			unique[trimmed] = true
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
