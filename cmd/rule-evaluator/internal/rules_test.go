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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type RuleGroupsRetrieverMock struct {
	RuleGroupsFunc    func() []*rules.Group
	AlertingRulesFunc func() []*rules.AlertingRule
}

func (r RuleGroupsRetrieverMock) RuleGroups() []*rules.Group {
	return r.RuleGroupsFunc()
}

func (r RuleGroupsRetrieverMock) AlertingRules() []*rules.AlertingRule {
	return r.AlertingRulesFunc()
}

func TestAPI_HandleRulesEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		rules                []*rules.Group
		req                  *http.Request
		expectedResponseCode int
		expectedResponseBody string
	}{
		{
			name:                 "match filters parameter not supported",
			req:                  httptest.NewRequest(http.MethodGet, rulesEndpoint+"?match[]=foo", nil),
			expectedResponseCode: http.StatusBadRequest,
			expectedResponseBody: `{"status":"error","errorType":"bad_data","error":"match[] parameter is not supported yet"}`,
		},
		{
			name:                 "invalid rule type filter",
			req:                  httptest.NewRequest(http.MethodGet, rulesEndpoint+"?"+typeFilterQueryParamName+"=foo", nil),
			expectedResponseCode: http.StatusBadRequest,
			expectedResponseBody: `{"status":"error","errorType":"bad_data","error":"invalid type parameter"}`,
		},
		{
			name:                 "invalid rule type filter",
			req:                  httptest.NewRequest(http.MethodGet, rulesEndpoint+"?"+typeFilterQueryParamName+"=foo", nil),
			expectedResponseCode: http.StatusBadRequest,
			expectedResponseBody: `{"status":"error","errorType":"bad_data","error":"invalid type parameter"}`,
		},
		{
			name:                 "invalid exclude_alerts parameter",
			req:                  httptest.NewRequest(http.MethodGet, rulesEndpoint+"?"+excludeAlertsQueryParam+"=foo", nil),
			expectedResponseCode: http.StatusBadRequest,
			expectedResponseBody: `{"status":"error","errorType":"bad_data","error":"invalid exclude_alerts parameter"}`,
		},
		{
			name:                 "happy path with empty groups",
			req:                  httptest.NewRequest(http.MethodGet, rulesEndpoint, nil),
			rules:                []*rules.Group{},
			expectedResponseCode: http.StatusOK,
			expectedResponseBody: `{"status":"success","data":{"groups":[]}}`,
		},
	}
	for _, tt := range tests {
		tt := tt //nolint:copyloopvar // parallel test
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &API{
				rulesManager: RuleGroupsRetrieverMock{RuleGroupsFunc: func() []*rules.Group { return tt.rules }},
				logger:       log.NewNopLogger(),
			}
			w := httptest.NewRecorder()
			api.HandleRulesEndpoint(w, tt.req)

			result := w.Result()
			defer result.Body.Close()

			data, err := io.ReadAll(result.Body)
			if err != nil {
				t.Errorf("Error: %v", err)
			}

			assert.Equal(t, tt.expectedResponseCode, result.StatusCode)
			require.JSONEq(t, tt.expectedResponseBody, string(data))
		})
	}
}

func TestAPI_groupsToAPIGroups(t *testing.T) {
	type args struct {
		groups       []*rules.Group
		fileFilters  []string
		groupFilters []string
	}
	tests := []struct {
		name string
		args args
		want []*apiv1.RuleGroup
	}{
		{
			name: "empty groups",
			args: args{
				groups:       []*rules.Group{},
				fileFilters:  []string{},
				groupFilters: []string{},
			},
			want: []*apiv1.RuleGroup{},
		},
		{
			name: "happy path with two groups",
			args: args{
				groups: []*rules.Group{
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-1",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-1", &parser.NumberLiteral{Val: 11}, []labels.Label{})},
					}),
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-2",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
				},
			},
			want: []*apiv1.RuleGroup{
				{Name: "test-group-1"},
				{Name: "test-group-2"},
			},
		},
		{
			name: "skips groups with no rules",
			args: args{
				groups: []*rules.Group{
					rules.NewGroup(rules.GroupOptions{
						Name: "test-group-1",
						Opts: &rules.ManagerOptions{},
					}),
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-2",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
				},
			},
			want: []*apiv1.RuleGroup{
				{Name: "test-group-2"},
			},
		},
		{
			name: "skips groups due to fileFilters parameter",
			args: args{
				groups: []*rules.Group{
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-1",
						File:  "test-file-1",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-2",
						File:  "test-file-2",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-3",
						File:  "test-file-1",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
				},
				fileFilters: []string{"test-file-1", "test-file-3"},
			},
			want: []*apiv1.RuleGroup{
				{Name: "test-group-1"},
				{Name: "test-group-3"},
			},
		},
		{
			name: "skips groups due to groupFilters parameter, and works with duplicates",
			args: args{
				groups: []*rules.Group{
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-1",
						File:  "test-file-1",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-2",
						File:  "test-file-2",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-1",
						File:  "test-file-1",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
					rules.NewGroup(rules.GroupOptions{
						Name:  "test-group-3",
						File:  "test-file-3",
						Opts:  &rules.ManagerOptions{},
						Rules: []rules.Rule{rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{})},
					}),
				},
				groupFilters: []string{"test-group-1"},
			},
			want: []*apiv1.RuleGroup{
				{Name: "test-group-1"},
				{Name: "test-group-1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &API{logger: log.NewNopLogger()}
			result := api.groupsToAPIGroups(tt.args.groups, nil, tt.args.fileFilters, tt.args.groupFilters, true, true, false)
			assert.Len(t, result, len(tt.want))
			for i := range result {
				assert.Equal(t, tt.want[i].Name, result[i].Name) // no need to deep-assert, test below do that already.
			}
		})
	}
}

func TestAPI_groupToAPIGroup(t *testing.T) {
	t.Parallel()
	type args struct {
		group                             *rules.Group
		ruleFilters                       []string
		shouldReturnAlertRules            bool
		shouldReturnRecordingRules        bool
		shouldExcludeAlertsFromAlertRules bool
	}
	tests := []struct {
		name string
		args args
		want *apiv1.RuleGroup
	}{
		{
			name: "empty group",
			args: args{
				group:                             &rules.Group{},
				ruleFilters:                       []string{},
				shouldReturnAlertRules:            true,
				shouldReturnRecordingRules:        true,
				shouldExcludeAlertsFromAlertRules: false,
			},
			want: &apiv1.RuleGroup{
				Name:           "",
				File:           "",
				Rules:          []apiv1.Rule{},
				Interval:       0,
				Limit:          0,
				EvaluationTime: 0,
				LastEvaluation: time.Time{},
			},
		},
		{
			name: "happy path with both rule types",
			args: args{
				group: rules.NewGroup(rules.GroupOptions{
					Name:     "test-group",
					File:     "test-file",
					Interval: time.Second * 10,
					Limit:    100,
					Opts:     &rules.ManagerOptions{},
					Rules: []rules.Rule{
						rules.NewRecordingRule("test-recording-1", &parser.NumberLiteral{Val: 11}, []labels.Label{{Name: "foo", Value: "bar"}}),
						rules.NewRecordingRule("test-recording-2", &parser.NumberLiteral{Val: 22}, []labels.Label{{Name: "bar", Value: "baz"}}),
						rules.NewAlertingRule("test-alert-1", &parser.NumberLiteral{Val: 33}, time.Hour, time.Hour*4, []labels.Label{{Name: "instance", Value: "localhost:9090"}}, []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}}, nil, "", false, log.NewNopLogger()),
						rules.NewAlertingRule("test-alert-2", &parser.NumberLiteral{Val: 44}, time.Hour*3, time.Hour*4, []labels.Label{{Name: "instance", Value: "localhost:9091"}}, []labels.Label{{Name: "summary", Value: "Test alert 2"}, {Name: "description", Value: "This is a test alert 2"}}, nil, "", false, log.NewNopLogger()),
					},
				}),
				ruleFilters:                       []string{},
				shouldReturnAlertRules:            true,
				shouldReturnRecordingRules:        true,
				shouldExcludeAlertsFromAlertRules: false,
			},
			want: &apiv1.RuleGroup{
				Name:           "test-group",
				File:           "test-file",
				Interval:       10,
				Limit:          100,
				EvaluationTime: 0,
				LastEvaluation: time.Time{},
				Rules: []apiv1.Rule{
					&apiv1.RecordingRule{Name: "test-recording-1", Query: "11", Labels: []labels.Label{{Name: "foo", Value: "bar"}}, Type: ruleKindRecording},
					&apiv1.RecordingRule{Name: "test-recording-2", Query: "22", Labels: []labels.Label{{Name: "bar", Value: "baz"}}, Type: ruleKindRecording},
					&apiv1.AlertingRule{
						State:         "inactive",
						Name:          "test-alert-1",
						Query:         "33",
						Duration:      3600,
						KeepFiringFor: 7200,
						Labels:        []labels.Label{{Name: "instance", Value: "localhost:9090"}},
						Annotations:   []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
						Alerts:        []*apiv1.Alert{},
						Health:        rules.HealthUnknown,
						Type:          ruleKindAlerting,
					},
					&apiv1.AlertingRule{
						State:         "inactive",
						Name:          "test-alert-2",
						Query:         "44",
						Duration:      10800,
						KeepFiringFor: 14400,
						Labels:        []labels.Label{{Name: "instance", Value: "localhost:9091"}},
						Annotations:   []labels.Label{{Name: "summary", Value: "Test alert 2"}, {Name: "description", Value: "This is a test alert 2"}},
						Alerts:        []*apiv1.Alert{},
						Health:        rules.HealthUnknown,
						Type:          ruleKindAlerting,
					},
				},
			},
		},
		{
			name: "skips rules due to ruleFilters parameter",
			args: args{
				group: rules.NewGroup(rules.GroupOptions{
					Name:     "test-group",
					File:     "test-file",
					Interval: time.Second * 10,
					Limit:    100,
					Opts:     &rules.ManagerOptions{},
					Rules: []rules.Rule{
						rules.NewRecordingRule("test-1", &parser.NumberLiteral{Val: 11}, []labels.Label{{Name: "foo", Value: "bar"}}),
						rules.NewRecordingRule("test-2", &parser.NumberLiteral{Val: 22}, []labels.Label{{Name: "bar", Value: "baz"}}),
						rules.NewRecordingRule("test-3", &parser.NumberLiteral{Val: 22}, []labels.Label{{Name: "baz", Value: "quo"}}),
						rules.NewAlertingRule("test-1", &parser.NumberLiteral{Val: 33}, time.Hour, time.Hour*4, nil, nil, nil, "", false, log.NewNopLogger()),
						rules.NewAlertingRule("test-2", &parser.NumberLiteral{Val: 44}, time.Hour*3, time.Hour*4, nil, nil, nil, "", false, log.NewNopLogger()),
						rules.NewAlertingRule("test-3", &parser.NumberLiteral{Val: 55}, time.Hour*3, time.Hour*4, nil, nil, nil, "", false, log.NewNopLogger()),
					},
				}),
				ruleFilters:                       []string{"test-2"},
				shouldReturnAlertRules:            true,
				shouldReturnRecordingRules:        true,
				shouldExcludeAlertsFromAlertRules: false,
			},
			want: &apiv1.RuleGroup{
				Name:           "test-group",
				File:           "test-file",
				Interval:       10,
				Limit:          100,
				EvaluationTime: 0,
				LastEvaluation: time.Time{},
				Rules: []apiv1.Rule{
					&apiv1.RecordingRule{Name: "test-2", Query: "22", Labels: []labels.Label{{Name: "bar", Value: "baz"}}, Type: ruleKindRecording},
					&apiv1.AlertingRule{
						State:          "inactive",
						Name:           "test-2",
						Query:          "44",
						Duration:       10800,
						KeepFiringFor:  14400,
						Labels:         []labels.Label{},
						Annotations:    []labels.Label{},
						Alerts:         []*apiv1.Alert{},
						Health:         rules.HealthUnknown,
						LastError:      "",
						EvaluationTime: 0,
						LastEvaluation: time.Time{},
						Type:           ruleKindAlerting,
					},
				},
			},
		},
		{
			name: "skips alert rules due to shouldReturnAlertRules parameter",
			args: args{
				group: rules.NewGroup(rules.GroupOptions{
					Name:     "test-group",
					File:     "test-file",
					Interval: time.Second * 10,
					Limit:    100,
					Opts:     &rules.ManagerOptions{},
					Rules: []rules.Rule{
						rules.NewRecordingRule("test-1", &parser.NumberLiteral{Val: 11}, []labels.Label{{Name: "foo", Value: "bar"}}),
						rules.NewRecordingRule("test-2", &parser.NumberLiteral{Val: 22}, []labels.Label{{Name: "bar", Value: "baz"}}),
						rules.NewAlertingRule("test-1", &parser.NumberLiteral{Val: 33}, time.Hour, time.Hour*4, nil, nil, nil, "", false, log.NewNopLogger()),
						rules.NewAlertingRule("test-2", &parser.NumberLiteral{Val: 44}, time.Hour*3, time.Hour*4, nil, nil, nil, "", false, log.NewNopLogger()),
					},
				}),
				ruleFilters:                       []string{},
				shouldReturnAlertRules:            false,
				shouldReturnRecordingRules:        true,
				shouldExcludeAlertsFromAlertRules: false,
			},
			want: &apiv1.RuleGroup{
				Name:           "test-group",
				File:           "test-file",
				Interval:       10,
				Limit:          100,
				EvaluationTime: 0,
				LastEvaluation: time.Time{},
				Rules: []apiv1.Rule{
					&apiv1.RecordingRule{Name: "test-1", Query: "11", Labels: []labels.Label{{Name: "foo", Value: "bar"}}, Type: ruleKindRecording},
					&apiv1.RecordingRule{Name: "test-2", Query: "22", Labels: []labels.Label{{Name: "bar", Value: "baz"}}, Type: ruleKindRecording},
				},
			},
		},
		{
			name: "skips recording rules due to shouldReturnRecordingRules parameter",
			args: args{
				group: rules.NewGroup(rules.GroupOptions{
					Name:     "test-group",
					File:     "test-file",
					Interval: time.Second * 10,
					Limit:    100,
					Opts:     &rules.ManagerOptions{},
					Rules: []rules.Rule{
						rules.NewRecordingRule("test-1", &parser.NumberLiteral{Val: 11}, []labels.Label{{Name: "foo", Value: "bar"}}),
						rules.NewRecordingRule("test-2", &parser.NumberLiteral{Val: 22}, []labels.Label{{Name: "bar", Value: "baz"}}),
						rules.NewAlertingRule("test-1", &parser.NumberLiteral{Val: 33}, 0, 0, nil, nil, nil, "", false, log.NewNopLogger()),
						rules.NewAlertingRule("test-2", &parser.NumberLiteral{Val: 44}, 0, 0, nil, nil, nil, "", false, log.NewNopLogger()),
					},
				}),
				ruleFilters:                       []string{},
				shouldReturnAlertRules:            true,
				shouldReturnRecordingRules:        false,
				shouldExcludeAlertsFromAlertRules: false,
			},
			want: &apiv1.RuleGroup{
				Name:           "test-group",
				File:           "test-file",
				Interval:       10,
				Limit:          100,
				EvaluationTime: 0,
				LastEvaluation: time.Time{},
				Rules: []apiv1.Rule{
					&apiv1.AlertingRule{
						State:       "inactive",
						Name:        "test-1",
						Query:       "33",
						Labels:      []labels.Label{},
						Annotations: []labels.Label{},
						Alerts:      []*apiv1.Alert{},
						Health:      rules.HealthUnknown,
						Type:        ruleKindAlerting,
					}, &apiv1.AlertingRule{
						State:       "inactive",
						Name:        "test-2",
						Query:       "44",
						Labels:      []labels.Label{},
						Annotations: []labels.Label{},
						Alerts:      []*apiv1.Alert{},
						Health:      rules.HealthUnknown,
						Type:        ruleKindAlerting,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt //nolint:copyloopvar // parallel test
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := &API{logger: log.NewNopLogger()}
			result := api.groupToAPIGroup(tt.args.group, tt.args.ruleFilters, tt.args.shouldReturnAlertRules, tt.args.shouldReturnRecordingRules, tt.args.shouldExcludeAlertsFromAlertRules)
			// deep Eval
			assert.Equal(t, tt.want.Name, result.Name)
			assert.Equal(t, tt.want.File, result.File)
			assert.Equal(t, tt.want.Interval, result.Interval) //nolint:testifylint // we want to assert exact equality, not delta equality here
			assert.Equal(t, tt.want.Limit, result.Limit)
			assert.Equal(t, tt.want.EvaluationTime, result.EvaluationTime) //nolint:testifylint // we want to assert exact equality, not delta equality here
			assert.Equal(t, tt.want.LastEvaluation, result.LastEvaluation)
			assert.Len(t, result.Rules, len(tt.want.Rules))
			for i := range result.Rules {
				switch rule := result.Rules[i].(type) {
				case *apiv1.RecordingRule:
					assert.Equal(t, tt.want.Rules[i].(*apiv1.RecordingRule).Name, rule.Name) // no need to deep-assert, test below do that already.
				case *apiv1.AlertingRule:
					assert.Equal(t, tt.want.Rules[i].(*apiv1.AlertingRule).Name, rule.Name)
				}
			}
		})
	}
}

func Test_recordingRuleToAPIRule(t *testing.T) {
	t.Parallel()
	timestamp := time.Date(1998, time.February, 1, 2, 3, 4, 567, time.UTC)
	rule := rules.NewRecordingRule("test-recording-1", &parser.NumberLiteral{Val: 13}, []labels.Label{{Name: "instance", Value: "localhost:9090"}})
	rule.SetLastError(fmt.Errorf("error for %s", "test-recording-1"))
	rule.SetEvaluationDuration(time.Second * 5)
	rule.SetEvaluationTimestamp(timestamp)

	expected := &apiv1.RecordingRule{
		Name:           "test-recording-1",
		Query:          "13",
		Labels:         []labels.Label{{Name: "instance", Value: "localhost:9090"}},
		LastError:      "error for test-recording-1",
		EvaluationTime: 5,
		LastEvaluation: timestamp,
		Type:           ruleKindRecording,
	}

	result := recordingRuleToAPIRule(rule)
	assert.Equal(t, expected.Name, result.Name)
}

func Test_alertingRuleToAPIRule(t *testing.T) {
	t.Parallel()
	timestamp := time.Date(1998, time.February, 1, 2, 3, 4, 567, time.UTC)

	rule := rules.NewAlertingRule(
		"test-alert-1",
		&parser.NumberLiteral{Val: 7},
		time.Hour,
		time.Hour*2,
		[]labels.Label{{Name: "instance", Value: "localhost:9090"}},
		[]labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
		nil, "", false, log.NewNopLogger(),
	)
	rule.SetHealth(rules.HealthGood)
	rule.SetLastError(fmt.Errorf("error for %s", "test-alert-1"))
	rule.SetEvaluationDuration(time.Second * 5)
	rule.SetEvaluationTimestamp(timestamp)

	expected := &apiv1.AlertingRule{
		State:          "inactive",
		Name:           "test-alert-1",
		Query:          "7",
		Duration:       3600,
		KeepFiringFor:  7200,
		Labels:         []labels.Label{{Name: "instance", Value: "localhost:9090"}},
		Annotations:    []labels.Label{{Name: "summary", Value: "Test alert 1"}, {Name: "description", Value: "This is a test alert"}},
		Alerts:         []*apiv1.Alert{},
		Health:         rules.HealthGood,
		LastError:      "error for test-alert-1",
		EvaluationTime: 5,
		LastEvaluation: timestamp,
		Type:           ruleKindAlerting,
	}

	assert.Equal(t, expected, alertingRuleToAPIRule(rule, false))
}
