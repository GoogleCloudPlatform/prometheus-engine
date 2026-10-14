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
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	grafana "github.com/grafana/grafana-api-golang-client"
	"github.com/prometheus/common/promslog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

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

func TestGetOAuth2Token(t *testing.T) {
	stale := &oauth2.Token{AccessToken: "stale", Expiry: time.Now().Add(time.Minute)}
	fresh := &oauth2.Token{AccessToken: "fresh", Expiry: time.Now().Add(time.Hour)}
	noExpiry := &oauth2.Token{AccessToken: "no-expiry"}

	tests := []struct {
		name      string
		tokens    []*oauth2.Token
		err       error
		want      string
		wantCalls int
		fail      bool
	}{
		{
			name:      "valid token on first attempt",
			tokens:    []*oauth2.Token{fresh},
			want:      "fresh",
			wantCalls: 1,
		},
		{
			name:      "token near expiry is fetched again",
			tokens:    []*oauth2.Token{stale, stale, fresh},
			want:      "fresh",
			wantCalls: 3,
		},
		{
			name:      "token without expiry is accepted",
			tokens:    []*oauth2.Token{noExpiry},
			want:      "no-expiry",
			wantCalls: 1,
		},
		{
			name:      "token source error",
			err:       errors.New("token error"),
			wantCalls: 1,
			fail:      true,
		},
		{
			name: "timeout while token stays near expiry",
			tokens: func() []*oauth2.Token {
				tokens := make([]*oauth2.Token, 1000)
				for i := range tokens {
					tokens[i] = stale
				}
				return tokens
			}(),
			fail: true,
		},
	}
	defer func(delay, timeout time.Duration) {
		tokenRetryDelay, tokenRetryTimeout = delay, timeout
		newDefaultTokenSource = google.DefaultTokenSource
	}(tokenRetryDelay, tokenRetryTimeout)
	tokenRetryDelay, tokenRetryTimeout = time.Millisecond, 100*time.Millisecond

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			newDefaultTokenSource = func(context.Context, ...string) (oauth2.TokenSource, error) {
				calls++
				if tt.err != nil {
					return nil, tt.err
				}
				return oauth2.StaticTokenSource(tt.tokens[calls-1]), nil
			}
			got, err := getOAuth2Token(context.Background(), promslog.NewNopLogger(), "")
			if tt.wantCalls != 0 && calls != tt.wantCalls {
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
			if got != tt.want {
				t.Errorf("got token %q, want %q", got, tt.want)
			}
		})
	}
}
