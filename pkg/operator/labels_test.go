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

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestResolveLabels(t *testing.T) {
	for _, tc := range []struct {
		desc                                  string
		projectID, location, cluster          string
		expProjectID, expLocation, expCluster string
		externalLabels                        map[string]string
		expExternalLabels                     map[string]string
	}{
		{
			desc:      "no overwrite",
			projectID: "proj-a",
			location:  "loc-a",
			cluster:   "clu-a",
			externalLabels: map[string]string{
				"project_id": "proj-b",
				"location":   "loc-b",
				"cluster":    "clu-b",
			},
			expProjectID: "proj-b",
			expLocation:  "loc-b",
			expCluster:   "clu-b",
			expExternalLabels: map[string]string{
				"project_id": "proj-b",
				"location":   "loc-b",
				"cluster":    "clu-b",
			},
		},
		{
			desc:      "overwrite projectID",
			projectID: "proj-a",
			location:  "loc-a",
			cluster:   "clu-a",
			externalLabels: map[string]string{
				"location": "loc-b",
				"cluster":  "clu-b",
			},
			expProjectID: "proj-a",
			expLocation:  "loc-b",
			expCluster:   "clu-b",
			expExternalLabels: map[string]string{
				"project_id": "proj-a",
				"location":   "loc-b",
				"cluster":    "clu-b",
			},
		},
		{
			desc:      "overwrite location",
			projectID: "proj-a",
			location:  "loc-a",
			cluster:   "clu-a",
			externalLabels: map[string]string{
				"project_id": "proj-b",
				"cluster":    "clu-b",
			},
			expProjectID: "proj-b",
			expLocation:  "loc-a",
			expCluster:   "clu-b",
			expExternalLabels: map[string]string{
				"project_id": "proj-b",
				"location":   "loc-a",
				"cluster":    "clu-b",
			},
		},
		{
			desc:      "overwrite cluster",
			projectID: "proj-a",
			location:  "loc-a",
			cluster:   "clu-a",
			externalLabels: map[string]string{
				"project_id": "proj-b",
				"location":   "loc-b",
			},
			expProjectID: "proj-b",
			expLocation:  "loc-b",
			expCluster:   "clu-a",
			expExternalLabels: map[string]string{
				"project_id": "proj-b",
				"location":   "loc-b",
				"cluster":    "clu-a",
			},
		},
		{
			desc:           "overwrite all",
			projectID:      "proj-a",
			location:       "loc-a",
			cluster:        "clu-a",
			externalLabels: map[string]string{},
			expProjectID:   "proj-a",
			expLocation:    "loc-a",
			expCluster:     "clu-a",
			expExternalLabels: map[string]string{
				"project_id": "proj-a",
				"location":   "loc-a",
				"cluster":    "clu-a",
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			opts := Options{
				ProjectID: tc.projectID,
				Location:  tc.location,
				Cluster:   tc.cluster,
			}
			projectID, location, cluster := resolveLabels(opts, tc.externalLabels)
			if projectID != tc.expProjectID {
				t.Error("projectIDs do not match")
			}
			if location != tc.expLocation {
				t.Error("locations do not match")
			}
			if cluster != tc.expCluster {
				t.Error("clusters do not match")
			}
			if diff := cmp.Diff(tc.expExternalLabels, tc.externalLabels); diff != "" {
				t.Errorf("unexpected externalLabels (-want, +got): %s", diff)
			}
		})
	}
}
