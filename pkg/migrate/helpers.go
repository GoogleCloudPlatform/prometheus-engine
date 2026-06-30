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
	"maps"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// protectedLabels contains the list of labels that are protected by GMP and cannot
// be overwritten by targetLabels or relabeling rules.
var protectedLabels = map[string]bool{
	"project_id":                true,
	"location":                  true,
	"cluster":                   true,
	"namespace":                 true,
	"job":                       true,
	"instance":                  true,
	"top_level_controller":      true,
	"top_level_controller_type": true,
	"__address__":               true,
}

// BuildTypeMeta constructs standard TypeMeta for a GMP resource Kind.
func BuildTypeMeta(kind string) metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: GMPAPIVersion,
		Kind:       kind,
	}
}

// CopyObjectMeta copies user-controlled metadata (Name, Labels, Annotations) from source to target,
// setting the namespace to the specified target namespace and filtering out helm and system annotations/labels.
func CopyObjectMeta(src metav1.ObjectMeta, targetNamespace string) metav1.ObjectMeta {
	dst := metav1.ObjectMeta{
		Name:        src.Name,
		Namespace:   targetNamespace,
		Labels:      maps.Clone(src.Labels),
		Annotations: maps.Clone(src.Annotations),
	}

	// Strip this annotation because it will contain the old PO schema.
	delete(dst.Annotations, "kubectl.kubernetes.io/last-applied-configuration")

	// Future consideration: Delete deployment specific annotations/labels (helm, ARGO, Kustomize etc.)

	return dst
}

// ParseAndCleanNamespaces trims whitespace, filters out empty strings, and deduplicates namespaces.
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
