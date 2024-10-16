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
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func TestClient_call(t *testing.T) {
	t.Parallel()

	type args struct {
		baseURL     url.URL
		endpoint    string
		queryString string
	}

	for _, tt := range []struct {
		name    string
		client  httpClient
		args    args
		want    string
		wantErr error
	}{
		{
			name: "happy path",
			client: &mockClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					require.Equal(t, "GET", req.Method)
					require.Equal(t, "http://example.edu/api/v1/test?queryParam=foo", req.URL.String())
					return &http.Response{
						Body:       io.NopCloser(strings.NewReader(`{"key": "value"}`)),
						StatusCode: http.StatusOK,
					}, nil
				},
			},
			args: args{
				baseURL:     url.URL{Scheme: "http", Host: "example.edu", Path: "/api/v1/"},
				endpoint:    "test",
				queryString: "queryParam=foo",
			},
			want:    `{"key": "value"}`,
			wantErr: nil,
		},
		{
			name: "error on non-2xx status code",
			client: &mockClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					return &http.Response{
						Body:       io.NopCloser(nil),
						StatusCode: http.StatusTeapot,
					}, nil
				},
			},
			wantErr: errRequestFailed,
		},
		{
			name: "error on client-response error",
			client: &mockClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					return nil, context.Canceled
				},
			},
			wantErr: context.Canceled,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newClient(tt.client)
			got, err := r.call(context.Background(), tt.args.baseURL, tt.args.endpoint, tt.args.queryString)

			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.want, string(got))
		})
	}
}

func TestClient_Alerts(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		client      httpClient
		baseURL     url.URL
		queryString string
		want        []*promapiv1.Alert
		wantErr     bool
	}{
		{
			name: "happy path",
			client: &mockClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					require.Equal(t, "GET", req.Method)
					require.Equal(t, "http://example.edu/path-prefix/api/v1/alerts?queryParam=foo", req.URL.String())
					return &http.Response{
						Body:       io.NopCloser(strings.NewReader(`{"status": "success", "data": {"alerts": [{"state": "firing"}]}}`)),
						StatusCode: http.StatusOK,
					}, nil
				},
			},
			baseURL:     url.URL{Scheme: "http", Host: "example.edu", Path: "/path-prefix"},
			queryString: "queryParam=foo",
			want:        []*promapiv1.Alert{{State: "firing"}},
		},
		{
			name: "json-unmarshal error results in error",
			client: &mockClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					return &http.Response{
						Body:       io.NopCloser(strings.NewReader(`not-json`)),
						StatusCode: http.StatusOK,
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "client-response error results in error",
			client: &mockClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					return nil, context.Canceled
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newClient(tt.client)
			got, err := r.Alerts(context.Background(), tt.baseURL, tt.queryString)

			require.Equal(t, tt.wantErr, err != nil)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestClient_Rules(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		client      httpClient
		baseURL     url.URL
		queryString string
		want        []*promapiv1.RuleGroup
		wantErr     bool
	}{
		{
			name: "happy path",
			client: &mockClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					require.Equal(t, "GET", req.Method)
					require.Equal(t, "http://example.edu/path-prefix/api/v1/rules?queryParam=bar", req.URL.String())
					return &http.Response{
						Body:       io.NopCloser(strings.NewReader(`{"status": "success", "data": {"groups": [{"name": "test"}]}}`)),
						StatusCode: http.StatusOK,
					}, nil
				},
			},
			baseURL:     url.URL{Scheme: "http", Host: "example.edu", Path: "/path-prefix"},
			queryString: "queryParam=bar",
			want:        []*promapiv1.RuleGroup{{Name: "test"}},
		},
		{
			name: "json-unmarshal error results in error",
			client: &mockClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					return &http.Response{
						Body:       io.NopCloser(strings.NewReader(`not-json`)),
						StatusCode: http.StatusOK,
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name: "client-response error results in error",
			client: &mockClient{
				DoFunc: func(_ *http.Request) (*http.Response, error) {
					return nil, context.Canceled
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newClient(tt.client)
			got, err := r.RuleGroups(context.Background(), tt.baseURL, tt.queryString)

			require.Equal(t, tt.wantErr, err != nil)
			require.Equal(t, tt.want, got)
		})
	}
}
