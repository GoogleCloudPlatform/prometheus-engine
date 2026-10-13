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

package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	grafana "github.com/grafana/grafana-api-golang-client"
	"golang.org/x/oauth2"

	"github.com/google/go-cmp/cmp"
)

var accessToken = "12345"

func TestBuildUpdateDataSourceRequest(t *testing.T) {
	tests := []struct {
		name  string
		input grafana.DataSource
		want  grafana.DataSource
		fail  bool
	}{
		{
			name: "OK",
			input: grafana.DataSource{
				Type:     "prometheus",
				JSONData: map[string]any{},
			},
			want: grafana.DataSource{
				URL:  "https://monitoring.googleapis.com/v1/projects/test/location/global/prometheus/",
				Type: "prometheus",
				JSONData: map[string]any{
					"httpHeaderName1":   "Authorization",
					"httpMethod":        "GET",
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.40.0",
					"queryTimeout":      "2m",
					"timeout":           "120",
				},
				SecureJSONData: map[string]any{
					"httpHeaderValue1": "Bearer 12345",
				},
			},
		},
		{
			name: "OK with adding extra httpHeaderName",
			input: grafana.DataSource{
				URL:  "http://localhost:9090",
				Type: "prometheus",
				JSONData: map[string]any{
					"httpHeaderName1": "X-Custom-Header",
					"httpHeaderName2": "X-Custom-Header2",
				},
			},
			want: grafana.DataSource{
				URL:  "https://monitoring.googleapis.com/v1/projects/test/location/global/prometheus/",
				Type: "prometheus",
				JSONData: map[string]any{
					"httpHeaderName1":   "X-Custom-Header",
					"httpHeaderName2":   "X-Custom-Header2",
					"httpHeaderName3":   "Authorization",
					"httpMethod":        "GET",
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.40.0",
					"queryTimeout":      "2m",
					"timeout":           "120",
				},
				SecureJSONData: map[string]any{
					"httpHeaderValue3": "Bearer 12345",
				},
			},
		},
		{
			name: "OK with editing headers",
			input: grafana.DataSource{
				URL:  "http://localhost:9090",
				Type: "prometheus",
				JSONData: map[string]any{
					"httpHeaderName1":   "X-Custom-Header",
					"httpHeaderName2":   "Authorization",
					"httpHeaderName3":   "X-Custom-Header3",
					"httpMethod":        "POST",
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.37.0",
				},
			},
			want: grafana.DataSource{
				URL:  "https://monitoring.googleapis.com/v1/projects/test/location/global/prometheus/",
				Type: "prometheus",
				JSONData: map[string]any{
					"httpHeaderName1":   "X-Custom-Header",
					"httpHeaderName2":   "Authorization",
					"httpHeaderName3":   "X-Custom-Header3",
					"httpMethod":        "GET",
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.40.0",
					"queryTimeout":      "2m",
					"timeout":           "120",
				},
				SecureJSONData: map[string]any{
					"httpHeaderValue2": "Bearer 12345",
				},
			},
		},
		{
			name: "prometheus server url override is reset and prometheus version upgraded to latest supported version",
			input: grafana.DataSource{
				Type: "prometheus",
				URL:  "http://localhost:9090",
				JSONData: map[string]any{
					"httpHeaderName1":   "X-Custom-Header",
					"httpHeaderName2":   "X-Custom-Header2",
					"httpHeaderName3":   "Authorization",
					"httpMethod":        "POST",
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.37.0",
				},
			},
			want: grafana.DataSource{
				URL:  "https://monitoring.googleapis.com/v1/projects/test/location/global/prometheus/",
				Type: "prometheus",
				JSONData: map[string]any{
					"httpHeaderName1":   "X-Custom-Header",
					"httpHeaderName2":   "X-Custom-Header2",
					"httpHeaderName3":   "Authorization",
					"httpMethod":        "GET",
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.40.0",
					"queryTimeout":      "2m",
					"timeout":           "120",
				},
				SecureJSONData: map[string]any{
					"httpHeaderValue3": "Bearer 12345",
				},
			},
		},
		{
			name: "prometheus version 2.40+ and editing data source fields is supported",
			input: grafana.DataSource{
				Type: "prometheus",
				URL:  "http://localhost:9090",
				JSONData: map[string]any{
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.42.0",
					"queryTimeout":      "3m",
					"timeout":           "160",
				},
			},
			want: grafana.DataSource{
				URL:  "https://monitoring.googleapis.com/v1/projects/test/location/global/prometheus/",
				Type: "prometheus",
				JSONData: map[string]any{
					"httpHeaderName1":   "Authorization",
					"httpMethod":        "GET",
					"prometheusType":    "Prometheus",
					"prometheusVersion": "2.42.0",
					"queryTimeout":      "3m",
					"timeout":           "160",
				},
				SecureJSONData: map[string]any{
					"httpHeaderValue1": "Bearer 12345",
				},
			},
		},
		{
			name: "non-prometheus data source type",
			input: grafana.DataSource{
				Type: "Cortex",
			},
			fail: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*projectID = "test"
			got, err := buildUpdateDataSourceRequest(tt.input, accessToken)
			if tt.fail {
				if err == nil {
					t.Fatal("unexpectedly succeeded")
				}
				return
			}
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("unmarshal gotJson failed with error: %v", err)
			}
			wantJSON, err := json.Marshal(tt.want)
			if err != nil {
				t.Fatalf("unmarshal wantJson failed with error: %v", err)
			}
			if diff := cmp.Diff(string(wantJSON), string(gotJSON)); diff != "" {
				t.Fatalf("unexpected json config (-want, +got): %s", diff)
			}
		})
	}
}

func TestFetchFreshToken(t *testing.T) {
	now := time.Now()
	stale := &oauth2.Token{AccessToken: "stale", Expiry: now.Add(time.Minute)}
	staler := &oauth2.Token{AccessToken: "staler", Expiry: now.Add(30 * time.Second)}
	fresh := &oauth2.Token{AccessToken: "fresh", Expiry: now.Add(time.Hour)}
	noExpiry := &oauth2.Token{AccessToken: "no-expiry"}
	errToken := errors.New("token error")

	tests := []struct {
		name      string
		tokens    []*oauth2.Token
		err       error
		attempts  int
		want      string
		wantCalls int
		fail      bool
	}{
		{
			name:      "fresh token on first attempt",
			attempts:  3,
			tokens:    []*oauth2.Token{fresh},
			want:      "fresh",
			wantCalls: 1,
		},
		{
			name:      "nearly expired token is fetched again",
			attempts:  3,
			tokens:    []*oauth2.Token{stale, fresh},
			want:      "fresh",
			wantCalls: 2,
		},
		{
			name:      "token without expiry is accepted",
			attempts:  3,
			tokens:    []*oauth2.Token{noExpiry},
			want:      "no-expiry",
			wantCalls: 1,
		},
		{
			name:      "token with the latest expiry is returned when attempts are exhausted",
			attempts:  3,
			tokens:    []*oauth2.Token{staler, stale, staler},
			want:      "stale",
			wantCalls: 3,
		},
		{
			name:      "token source error",
			attempts:  3,
			err:       errToken,
			wantCalls: 1,
			fail:      true,
		},
		{
			name:      "token source returns no token",
			attempts:  3,
			tokens:    []*oauth2.Token{nil},
			wantCalls: 1,
			fail:      true,
		},
		{
			name:      "no attempts",
			attempts:  0,
			wantCalls: 0,
			fail:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			newTokenSource := func() (oauth2.TokenSource, error) {
				if tt.err != nil {
					calls++
					return nil, tt.err
				}
				token := tt.tokens[calls]
				calls++
				return nilableTokenSource{token}, nil
			}
			got, err := fetchFreshToken(newTokenSource, 5*time.Minute, tt.attempts, 0)
			if calls != tt.wantCalls {
				t.Errorf("got %d token fetches, want %d", calls, tt.wantCalls)
			}
			if tt.fail {
				if err == nil {
					t.Fatal("unexpectedly succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.AccessToken != tt.want {
				t.Errorf("got token %q, want %q", got.AccessToken, tt.want)
			}
		})
	}
}

// nilableTokenSource returns its token as-is, unlike oauth2.StaticTokenSource
// which never returns a nil token.
type nilableTokenSource struct {
	token *oauth2.Token
}

func (s nilableTokenSource) Token() (*oauth2.Token, error) {
	return s.token, nil
}
