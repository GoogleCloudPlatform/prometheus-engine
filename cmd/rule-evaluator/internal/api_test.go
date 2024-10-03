// Copyright 2024 Google LLC
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

package internal

import (
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/rules"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/stretchr/testify/assert"
)

func Test_rulesAlertsToAPIAlerts(t *testing.T) {
	t.Parallel()
	location, _ := time.LoadLocation("Europe/Stockholm")
	activeAt := time.Date(1998, time.February, 1, 2, 3, 4, 567, location)
	keepFiringSince := activeAt.Add(-time.Hour)

	tests := []struct {
		name        string
		rulesAlerts []*rules.Alert
		want        []*apiv1.Alert
	}{
		{
			name:        "empty rules alerts",
			rulesAlerts: []*rules.Alert{},
			want:        []*apiv1.Alert{},
		},
		{
			name: "happy path with two alerts",
			rulesAlerts: []*rules.Alert{
				{
					Labels:          []labels.Label{{Name: "alertname", Value: "test-alert-1"}, {Name: "instance", Value: "localhost:9090"}},
					Annotations:     []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
					State:           rules.StateFiring,
					ActiveAt:        activeAt,
					KeepFiringSince: keepFiringSince,
					Value:           1.23,
				},
				{
					Labels:          []labels.Label{{Name: "alertname", Value: "test-alert-1"}, {Name: "instance", Value: "localhost:9090"}},
					Annotations:     []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
					State:           rules.StatePending,
					ActiveAt:        activeAt,
					KeepFiringSince: keepFiringSince,
					Value:           1234234.24,
				},
			},
			want: []*apiv1.Alert{
				{
					Labels:          []labels.Label{{Name: "alertname", Value: "test-alert-1"}, {Name: "instance", Value: "localhost:9090"}},
					Annotations:     []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
					State:           "firing",
					ActiveAt:        &activeAt,
					KeepFiringSince: &keepFiringSince,
					Value:           "1.23e+00",
				},
				{
					Labels:          []labels.Label{{Name: "alertname", Value: "test-alert-1"}, {Name: "instance", Value: "localhost:9090"}},
					Annotations:     []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
					State:           rules.StatePending.String(),
					ActiveAt:        &activeAt,
					KeepFiringSince: &keepFiringSince,
					Value:           "1.23423424e+06",
				},
			},
		},
		{
			name: "handlesZeroTime",
			rulesAlerts: []*rules.Alert{
				{
					Labels:          []labels.Label{{Name: "alertname", Value: "test-alert-1"}, {Name: "instance", Value: "localhost:9090"}},
					Annotations:     []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
					State:           rules.StateFiring,
					ActiveAt:        activeAt,
					KeepFiringSince: time.Time{},
					Value:           1.23,
				},
			},
			want: []*apiv1.Alert{
				{
					Labels:          []labels.Label{{Name: "alertname", Value: "test-alert-1"}, {Name: "instance", Value: "localhost:9090"}},
					Annotations:     []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
					State:           "firing",
					ActiveAt:        &activeAt,
					KeepFiringSince: nil,
					Value:           "1.23e+00",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := alertsToAPIAlerts(tt.rulesAlerts)
			assert.Len(t, result, len(tt.want))
			for i := range result {
				assert.Equal(t, tt.want[i].Labels, result[i].Labels)
				assert.Equal(t, tt.want[i].Annotations, result[i].Annotations)
				assert.Equal(t, tt.want[i].State, result[i].State)
				assert.Equal(t, tt.want[i].ActiveAt, result[i].ActiveAt)
				assert.Equal(t, tt.want[i].KeepFiringSince, result[i].KeepFiringSince)
				assert.Equal(t, tt.want[i].Value, result[i].Value)
			}
		})
	}
}
