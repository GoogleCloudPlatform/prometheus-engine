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

import "github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"

// resolveLabels compares the project, location, and cluster labels used by
// the operator against those in externalLabels. If any are found in the
// latter, they take precedence and are returned. If any are not found, they are
// inserted into the externalLabels.
// The higher-precedence labels are then returned.
func resolveLabels(opts Options, exLabels *map[string]string) (projectID, location, cluster string) {
	var externalLabels map[string]string
	if exLabels == nil {
		externalLabels = make(map[string]string)
	}
	externalLabels = *exLabels
	// Prioritize OperatorConfig's external labels over operator's flags
	// to be consistent with our export layer's priorities.
	// This is to avoid confusion if users specify a project_id, location, and
	// cluster in the OperatorConfig's external labels but not in flags passed
	// to the operator - since on GKE environnments, these values are autopopulated
	// without user intervention.
	if _, ok := externalLabels[export.KeyProjectID]; !ok {
		externalLabels[export.KeyProjectID] = opts.ProjectID
	}
	if _, ok := externalLabels[export.KeyLocation]; !ok {
		externalLabels[export.KeyLocation] = opts.Location
	}
	if _, ok := externalLabels[export.KeyCluster]; !ok {
		externalLabels[export.KeyCluster] = opts.Cluster
	}

	projectID = externalLabels[export.KeyProjectID]
	location = externalLabels[export.KeyLocation]
	cluster = externalLabels[export.KeyCluster]
	return
}
