// Copyright 2022 Google LLC
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

package operator

import "github.com/GoogleCloudPlatform/prometheus-engine/collector/export"

// resolveLabels compares the project, location, and cluster labels used by the operator
// against those in externalLabels. If any are found in the latter, they take precedence.
// The higher-precedence labels are then returned.
//
// This is to be consistent with our export layer's priorities and avoid confusion if users
// specify a project_id, location, and cluster in the OperatorConfig's external labels but
// not in flags passed to the operator - since on GKE environments, these values are
// auto-populated without user intervention.
func resolveLabels(defaultProjectID, defaultLocation, defaultCluster string, externalLabels map[string]string) (projectID, location, cluster string) {
	if externalLabels == nil {
		return defaultProjectID, defaultLocation, defaultCluster
	}

	var ok bool
	if projectID, ok = externalLabels[export.KeyProjectID]; !ok {
		projectID = defaultProjectID
	}
	if location, ok = externalLabels[export.KeyLocation]; !ok {
		location = defaultLocation
	}
	if cluster, ok = externalLabels[export.KeyCluster]; !ok {
		cluster = defaultCluster
	}
	return projectID, location, cluster
}
