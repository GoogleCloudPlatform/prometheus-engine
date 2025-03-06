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

package rule

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/internal/promapi"
	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/model/labels"
	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/stretchr/testify/require"
)

type mockRetriever struct {
	AlertsFunc     func(context.Context, url.URL, string) ([]*promapiv1.Alert, error)
	RuleGroupsFunc func(context.Context, url.URL, string) ([]*promapiv1.RuleGroup, error)
}

func (m *mockRetriever) Alerts(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.Alert, error) {
	return m.AlertsFunc(ctx, baseURL, queryString)
}

func (m *mockRetriever) RuleGroups(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.RuleGroup, error) {
	return m.RuleGroupsFunc(ctx, baseURL, queryString)
}

func TestProxy_handleError(t *testing.T) {
	t.Parallel()

	dummyRequest, _ := http.NewRequest(http.MethodGet, "http://localhost/path", nil)

	for _, tt := range []struct {
		name          string
		err           error
		wantStatus    int
		wantErrorType string
	}{
		{
			name:          "context canceled returns canceled error-type and 504",
			err:           context.Canceled,
			wantStatus:    http.StatusGatewayTimeout,
			wantErrorType: string(promapi.ErrorCanceled),
		},
		{
			name:          "context deadline exceeded returns timeout error-type and 504",
			err:           context.DeadlineExceeded,
			wantStatus:    http.StatusGatewayTimeout,
			wantErrorType: string(promapi.ErrorTimeout),
		},
		{
			name:          "generic error returns internal error-type and 500",
			err:           errors.New("some error"),
			wantStatus:    http.StatusInternalServerError,
			wantErrorType: string(promapi.ErrorInternal),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			p := NewProxy(promslog.NewNopLogger(), nil, nil)
			p.handleError(recorder, dummyRequest, tt.err)

			require.Equal(t, tt.wantStatus, recorder.Code)

			response := promapi.Response[promapi.GenericResponseData]{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			require.NoError(t, err)
			require.Equal(t, promapi.ErrorType(tt.wantErrorType), response.ErrorType)
		})
	}
}

func TestFanoutForward_AlertsReturnSuccess(t *testing.T) {
	t.Parallel()

	mockCli := &mockClient{
		DoFunc: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(`{"status":"success","data":{"alerts":[{"labels":{"labelKey1":"labelVal1"},"annotations":{"annoKey1":"AnnoVal1"},"state":"firing","activeAt":"2011-11-11T11:11:11.111122223Z","value":"1e+00"},{"labels":{"labelKey2":"labelVal2"},"annotations":{"annoKey2":"AnnoVal2"},"state":"firing","activeAt":"2022-02-22T22:22:22.999977773Z","value":"2e+00"}]}}`)),
				StatusCode: http.StatusOK,
			}, nil
		},
	}
	retriever := newClient(mockCli)

	activeAt1, _ := time.Parse(time.RFC3339Nano, "2011-11-11T11:11:11.111122223Z")
	activeAt2, _ := time.Parse(time.RFC3339Nano, "2022-02-22T22:22:22.999977773Z")
	expected := []*promapiv1.Alert{ // 2 times called a client which each returned 2 alerts ==> 4 alerts
		{
			Labels:          []labels.Label{{Name: "labelKey1", Value: "labelVal1"}},
			Annotations:     []labels.Label{{Name: "annoKey1", Value: "AnnoVal1"}},
			State:           "firing",
			ActiveAt:        &activeAt1,
			Value:           "1e+00",
			KeepFiringSince: nil,
		},
		{
			Labels:          []labels.Label{{Name: "labelKey2", Value: "labelVal2"}},
			Annotations:     []labels.Label{{Name: "annoKey2", Value: "AnnoVal2"}},
			State:           "firing",
			ActiveAt:        &activeAt2,
			Value:           "2e+00",
			KeepFiringSince: nil,
		},
		{
			Labels:          []labels.Label{{Name: "labelKey1", Value: "labelVal1"}},
			Annotations:     []labels.Label{{Name: "annoKey1", Value: "AnnoVal1"}},
			State:           "firing",
			ActiveAt:        &activeAt1,
			Value:           "1e+00",
			KeepFiringSince: nil,
		},
		{
			Labels:          []labels.Label{{Name: "labelKey2", Value: "labelVal2"}},
			Annotations:     []labels.Label{{Name: "annoKey2", Value: "AnnoVal2"}},
			State:           "firing",
			ActiveAt:        &activeAt2,
			Value:           "2e+00",
			KeepFiringSince: nil,
		},
	}

	retrieverUrls := []url.URL{
		{Scheme: "http", Host: "localhost:8080", Path: "with-prefix"},
		{Scheme: "https", Host: "localhost:8081", Path: ""},
	}

	alerts, err := fanoutForward(t.Context(), promslog.NewNopLogger(), retrieverUrls, "?qkey=qval", func(ctx context.Context, u url.URL, s string) ([]*promapiv1.Alert, error) {
		return retriever.Alerts(ctx, u, s)
	})

	require.NoError(t, err)
	require.Len(t, alerts, 4)
	require.Equal(t, expected, alerts)
}

func TestFanoutForward_AlertsTwoReturnSuccessWithOneOfTwoBrokenClients(t *testing.T) {
	t.Parallel()

	retrieverUrls := []url.URL{
		{Scheme: "http", Host: "localhost:8080", Path: "with-prefix"},
		{Scheme: "https", Host: "localhost:8081", Path: ""},
	}

	mockCli := &mockClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "localhost:8080" {
				return nil, errors.New("some error")
			}
			return &http.Response{
				Body:       io.NopCloser(strings.NewReader(`{"status":"success","data":{"alerts":[{"labels":{"labelKey1":"labelVal1"},"annotations":{"annoKey1":"AnnoVal1"},"state":"firing","activeAt":"2011-11-11T11:11:11.111122223Z","value":"1e+00"},{"labels":{"labelKey2":"labelVal2"},"annotations":{"annoKey2":"AnnoVal2"},"state":"firing","activeAt":"2022-02-22T22:22:22.999977773Z","value":"2e+00"}]}}`)),
				StatusCode: http.StatusOK,
			}, nil
		},
	}
	retriever := newClient(mockCli)

	activeAt1, _ := time.Parse(time.RFC3339Nano, "2011-11-11T11:11:11.111122223Z")
	activeAt2, _ := time.Parse(time.RFC3339Nano, "2022-02-22T22:22:22.999977773Z")
	expected := []*promapiv1.Alert{ // 2 times called a client which each returned 2 alerts ==> 4 alerts
		{
			Labels:          []labels.Label{{Name: "labelKey1", Value: "labelVal1"}},
			Annotations:     []labels.Label{{Name: "annoKey1", Value: "AnnoVal1"}},
			State:           "firing",
			ActiveAt:        &activeAt1,
			Value:           "1e+00",
			KeepFiringSince: nil,
		},
		{
			Labels:          []labels.Label{{Name: "labelKey2", Value: "labelVal2"}},
			Annotations:     []labels.Label{{Name: "annoKey2", Value: "AnnoVal2"}},
			State:           "firing",
			ActiveAt:        &activeAt2,
			Value:           "2e+00",
			KeepFiringSince: nil,
		},
	}
	alerts, err := fanoutForward(t.Context(), promslog.NewNopLogger(), retrieverUrls, "?qkey=qval", func(ctx context.Context, u url.URL, s string) ([]*promapiv1.Alert, error) {
		return retriever.Alerts(ctx, u, s)
	})

	require.NoError(t, err)
	require.Len(t, alerts, 2)
	require.Equal(t, expected, alerts)
}

func TestFanoutForward_AlertsTwoReturnErrorIfAllClientsFail(t *testing.T) {
	t.Parallel()

	retrieverUrls := []url.URL{
		{Scheme: "http", Host: "localhost:8080", Path: "with-prefix"},
		{Scheme: "https", Host: "localhost:8081", Path: ""},
	}

	mockCli := &mockClient{
		DoFunc: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("some error")
		},
	}
	retriever := newClient(mockCli)
	alerts, err := fanoutForward(t.Context(), promslog.NewNopLogger(), retrieverUrls, "?qkey=qval", func(ctx context.Context, u url.URL, s string) ([]*promapiv1.Alert, error) {
		return retriever.Alerts(ctx, u, s)
	})

	require.Nil(t, alerts)
	require.ErrorIs(t, err, errAllEndpointsFailed)
}

func TestProxy_Alerts(t *testing.T) {
	t.Parallel()

	activeAt1, _ := time.Parse(time.RFC3339Nano, "2011-11-11T11:11:11.111122223Z")
	activeAt2, _ := time.Parse(time.RFC3339Nano, "2022-02-22T22:22:22.999977773Z")
	for _, tt := range []struct {
		name                  string
		ruleEvaluatorBaseURLs []url.URL
		ruleRetriever         retriever
		wantStatus            int
		wantBody              string
	}{
		{
			name:                  "no rule evaluators returns success with empty alerts",
			ruleEvaluatorBaseURLs: []url.URL{},
			ruleRetriever: &mockRetriever{
				AlertsFunc: func(context.Context, url.URL, string) ([]*promapiv1.Alert, error) {
					t.Fatal("Should not call the rule retriever if there are no rule evaluators' URLs")
					return nil, nil
				},
				RuleGroupsFunc: func(context.Context, url.URL, string) ([]*promapiv1.RuleGroup, error) {
					t.Fatal("Should not call the rule retriever if there are no rule evaluators' URLs")
					return nil, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"success","data":{"alerts":[]}}`,
		},
		{
			name: "func calls correct client method",
			ruleEvaluatorBaseURLs: []url.URL{
				{Scheme: "http", Host: "localhost:8080", Path: "with-prefix"},
			},
			ruleRetriever: &mockRetriever{
				RuleGroupsFunc: func(context.Context, url.URL, string) ([]*promapiv1.RuleGroup, error) {
					t.Fatal("Should not call the RULES endpoint when fetching alerts")
					return nil, nil
				},
				AlertsFunc: func(_ context.Context, baseURL url.URL, _ string) ([]*promapiv1.Alert, error) {
					require.Equal(t, "http://localhost:8080/with-prefix", baseURL.String())
					return []*promapiv1.Alert{
						{
							Labels:      []labels.Label{{Name: "labelKey1", Value: "labelVal1"}},
							Annotations: []labels.Label{{Name: "annoKey1", Value: "AnnoVal1"}},
							State:       "firing",
							ActiveAt:    &activeAt1,
							Value:       "1e+00",
						},
						{
							Labels:      []labels.Label{{Name: "labelKey2", Value: "labelVal2"}},
							Annotations: []labels.Label{{Name: "annoKey2", Value: "AnnoVal2"}},
							State:       "firing",
							ActiveAt:    &activeAt2,
							Value:       "2e+00",
						},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"success","data":{"alerts":[{"labels":{"labelKey1":"labelVal1"},"annotations":{"annoKey1":"AnnoVal1"},"state":"firing","activeAt":"2011-11-11T11:11:11.111122223Z","value":"1e+00"},{"labels":{"labelKey2":"labelVal2"},"annotations":{"annoKey2":"AnnoVal2"},"state":"firing","activeAt":"2022-02-22T22:22:22.999977773Z","value":"2e+00"}]}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &Proxy{
				logger:    promslog.NewNopLogger(),
				endpoints: tt.ruleEvaluatorBaseURLs,
				client:    tt.ruleRetriever,
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			w := httptest.NewRecorder()
			r.Alerts(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.JSONEqf(t, tt.wantBody, w.Body.String(), "expected: %s, got: %s", tt.wantBody, w.Body.String())
		})
	}
}

func TestProxy_RuleGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		ruleEvaluatorBaseURLs []url.URL
		ruleRetriever         retriever
		wantStatus            int
		wantBody              string
	}{
		{
			name:                  "no rule evaluators returns success with empty groups",
			ruleEvaluatorBaseURLs: []url.URL{},
			ruleRetriever: &mockRetriever{
				AlertsFunc: func(context.Context, url.URL, string) ([]*promapiv1.Alert, error) {
					t.Fatal("Should not call the rule retriever if there are no rule evaluators' URLs")
					return nil, nil
				},
				RuleGroupsFunc: func(context.Context, url.URL, string) ([]*promapiv1.RuleGroup, error) {
					t.Fatal("Should not call the rule retriever if there are no rule evaluators' URLs")
					return nil, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"success","data":{"groups":[]}}`,
		},
		{
			name: "func calls correct client method",
			ruleEvaluatorBaseURLs: []url.URL{
				{Scheme: "http", Host: "localhost:8080", Path: "with-prefix"},
			},
			ruleRetriever: &mockRetriever{
				AlertsFunc: func(context.Context, url.URL, string) ([]*promapiv1.Alert, error) {
					t.Fatal("Should not call the ALERTS endpoint when fetching rules")
					return nil, nil
				},
				RuleGroupsFunc: func(_ context.Context, baseURL url.URL, _ string) ([]*promapiv1.RuleGroup, error) {
					require.Equal(t, "http://localhost:8080/with-prefix", baseURL.String())
					return []*promapiv1.RuleGroup{
						{
							Name:  "group1",
							File:  "file1",
							Rules: []promapiv1.Rule{},
						},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"success","data":{"groups":[{"name":"group1","file":"file1","rules":[],"interval":0,"limit":0,"evaluationTime":0,"lastEvaluation":"0001-01-01T00:00:00Z"}]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Proxy{
				logger:    promslog.NewNopLogger(),
				endpoints: tt.ruleEvaluatorBaseURLs,
				client:    tt.ruleRetriever,
			}

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			w := httptest.NewRecorder()
			r.RuleGroups(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.JSONEqf(t, tt.wantBody, w.Body.String(), "expected: %s, got: %s", tt.wantBody, w.Body.String())
		})
	}
}
