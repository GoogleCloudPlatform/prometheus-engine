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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_HandleAlertsEndpoint(t *testing.T) {
	t.Parallel()

	api := &API{
		rulesManager: RuleGroupsRetrieverMock{
			AlertingRulesFunc: func() []*rules.AlertingRule {
				return []*rules.AlertingRule{
					rules.NewAlertingRule("test-alert-1", &parser.NumberLiteral{Val: 33}, time.Hour, time.Hour*4, []labels.Label{{Name: "instance", Value: "localhost:9090"}}, []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}}, nil, "", false, promslog.NewNopLogger()),
					rules.NewAlertingRule("test-alert-2", &parser.NumberLiteral{Val: 33}, time.Hour, time.Hour*4, []labels.Label{{Name: "instance", Value: "localhost:9090"}}, []labels.Label{{Name: "summary", Value: "Test alert 2"}, {Name: "description", Value: "This is another test alert"}}, nil, "", false, promslog.NewNopLogger()),
				}
			},
		},
		logger: promslog.NewNopLogger(),
	}
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)

	api.HandleAlertsEndpoint(w, req)

	result := w.Result()
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		t.Errorf("Error: %v", err)
	}

	assert.Equal(t, http.StatusOK, result.StatusCode)
	require.JSONEq(t, `{"status":"success","data":{"alerts":[]}}`, string(data))
}
