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
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_HandleAlertsEndpoint(t *testing.T) {
	t.Parallel()

	newAlertRule := func(name string) *rules.AlertingRule {
		return rules.NewAlertingRule(name, &parser.NumberLiteral{Val: 33}, time.Hour, time.Hour*4, []labels.Label{{Name: "instance", Value: "localhost:9090"}}, []labels.Label{{Name: "summary", Value: "Test alert"}, {Name: "description", Value: "This is a test alert"}}, nil, "", false, log.NewNopLogger())
	}

	// Alerting rule with active (firing) alerts.
	newFiringAlertRule := func(name string) *rules.AlertingRule {
		a := newAlertRule(name)

		ts, _ := time.Parse(time.RFC3339Nano, "2025-04-11T14:03:59.791816+01:00")
		// AlertingRule does not allow injecting active alerts, so we use Eval with a fake querier
		// that always return 2 series, which will cause alerting rule to contain 2 active alerts.
		_, err := a.Eval(t.Context(), 0*time.Second, ts, func(context.Context, string, time.Time) (promql.Vector, error) {
			return promql.Vector{
				promql.Sample{T: timestamp.FromTime(ts), F: 10, Metric: labels.FromStrings("foo", "bar")},
				promql.Sample{T: timestamp.FromTime(ts), F: 11, Metric: labels.FromStrings("foo", "bar2")},
			}, nil
		}, nil, 0)
		require.NoError(t, err)
		return a
	}

	logger := log.NewNopLogger()
	for _, tcase := range []struct {
		name          string
		alertingRules []*rules.AlertingRule
		expectedJSON  string
	}{
		{
			name:          "no alerts",
			alertingRules: []*rules.AlertingRule{},
			expectedJSON:  `{"status":"success","data":{"alerts":[]}}`,
		},
		{
			name: "no firing alerts",
			alertingRules: []*rules.AlertingRule{
				newAlertRule("test-alert-1"),
				newAlertRule("test-alert-2"),
			},
			// Alert API returns only active alerts.
			expectedJSON: `{"status":"success","data":{"alerts":[]}}`,
		},
		{
			name: "mix of firing and not-firing alerts",
			alertingRules: []*rules.AlertingRule{
				newAlertRule("test-alert-1"),
				newFiringAlertRule("test-alert-2"),
			},
			expectedJSON: `{"status":"success","data":{"alerts":[{"labels":{"alertname":"test-alert-2","foo":"bar2","instance":"localhost:9090"},"annotations":{"description":"This is a test alert","summary":"Test alert"},"state":"pending","activeAt":"2025-04-11T14:03:59.791816+01:00","value":"1.1e+01"},{"labels":{"alertname":"test-alert-2","foo":"bar","instance":"localhost:9090"},"annotations":{"description":"This is a test alert","summary":"Test alert"},"state":"pending","activeAt":"2025-04-11T14:03:59.791816+01:00","value":"1e+01"}]}}`,
		},
		{
			name: "only firing alerts",
			alertingRules: []*rules.AlertingRule{
				newFiringAlertRule("test-alert-1"),
				newFiringAlertRule("test-alert-2"),
			},
			expectedJSON: `{"status":"success","data":{"alerts":[{"labels":{"alertname":"test-alert-1","foo":"bar2","instance":"localhost:9090"},"annotations":{"description":"This is a test alert","summary":"Test alert"},"state":"pending","activeAt":"2025-04-11T14:03:59.791816+01:00","value":"1.1e+01"},{"labels":{"alertname":"test-alert-1","foo":"bar","instance":"localhost:9090"},"annotations":{"description":"This is a test alert","summary":"Test alert"},"state":"pending","activeAt":"2025-04-11T14:03:59.791816+01:00","value":"1e+01"},{"labels":{"alertname":"test-alert-2","foo":"bar2","instance":"localhost:9090"},"annotations":{"description":"This is a test alert","summary":"Test alert"},"state":"pending","activeAt":"2025-04-11T14:03:59.791816+01:00","value":"1.1e+01"},{"labels":{"alertname":"test-alert-2","foo":"bar","instance":"localhost:9090"},"annotations":{"description":"This is a test alert","summary":"Test alert"},"state":"pending","activeAt":"2025-04-11T14:03:59.791816+01:00","value":"1e+01"}]}}`,
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			t.Parallel()

			api := &API{
				rulesManager: RuleGroupsRetrieverMock{
					AlertingRulesFunc: func() []*rules.AlertingRule { return tcase.alertingRules },
				},
				logger: logger,
			}
			w := httptest.NewRecorder()
			api.HandleAlertsEndpoint(w, httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil))

			result := w.Result()
			defer result.Body.Close()

			data, err := io.ReadAll(result.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, result.StatusCode)
			require.JSONEq(t, tcase.expectedJSON, string(data))
		})
	}
}
