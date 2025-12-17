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
	"testing"

	grafana "github.com/grafana/grafana-api-golang-client"

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
