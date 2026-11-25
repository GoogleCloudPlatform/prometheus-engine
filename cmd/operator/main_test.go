// Copyright 2026 Google LLC
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

package main

import (
	"context"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
)

type fakeMetadataProvider struct {
	onGCE           bool
	projectID       string
	projectIDErr    error
	attrs           map[string]string
	attrErrs        map[string]error
	onGCECalls      int
	projectIDCalls  int
	attrCalls       []string
	observedTimeout bool
}

func (f *fakeMetadataProvider) OnGCEWithContext(ctx context.Context) bool {
	f.onGCECalls++
	f.recordContext(ctx)
	return f.onGCE
}

func (f *fakeMetadataProvider) ProjectIDWithContext(ctx context.Context) (string, error) {
	f.projectIDCalls++
	f.recordContext(ctx)
	return f.projectID, f.projectIDErr
}

func (f *fakeMetadataProvider) InstanceAttributeValueWithContext(ctx context.Context, attr string) (string, error) {
	f.attrCalls = append(f.attrCalls, attr)
	f.recordContext(ctx)
	if err, ok := f.attrErrs[attr]; ok {
		return "", err
	}
	return f.attrs[attr], nil
}

func (f *fakeMetadataProvider) recordContext(ctx context.Context) {
	if _, ok := ctx.Deadline(); ok {
		f.observedTimeout = true
	}
}

func TestPopulateMetadataDefaults(t *testing.T) {
	tests := []struct {
		name               string
		projectID          string
		cluster            string
		location           string
		md                 *fakeMetadataProvider
		wantProjectID      string
		wantCluster        string
		wantLocation       string
		wantOnGCECalls     int
		wantProjectIDCalls int
		wantAttrCalls      int
		wantErr            bool
	}{
		{
			name:      "all flags set skips GCE metadata entirely",
			projectID: "flag-proj",
			cluster:   "flag-cluster",
			location:  "flag-loc",
			md: &fakeMetadataProvider{
				onGCE:     true,
				projectID: "gce-proj",
				attrs: map[string]string{
					"cluster-name":     "gce-cluster",
					"cluster-location": "gce-loc",
				},
			},
			wantProjectID:      "flag-proj",
			wantCluster:        "flag-cluster",
			wantLocation:       "flag-loc",
			wantOnGCECalls:     0,
			wantProjectIDCalls: 0,
			wantAttrCalls:      0,
		},
		{
			name:      "empty flags populated from GCE metadata",
			projectID: "",
			cluster:   "",
			location:  "",
			md: &fakeMetadataProvider{
				onGCE:     true,
				projectID: "gce-proj",
				attrs: map[string]string{
					"cluster-name":     "gce-cluster",
					"cluster-location": "gce-loc",
				},
			},
			wantProjectID:      "gce-proj",
			wantCluster:        "gce-cluster",
			wantLocation:       "gce-loc",
			wantOnGCECalls:     1,
			wantProjectIDCalls: 1,
			wantAttrCalls:      2,
		},
		{
			name:      "partially set flags only query missing metadata",
			projectID: "flag-proj",
			cluster:   "",
			location:  "flag-loc",
			md: &fakeMetadataProvider{
				onGCE:     true,
				projectID: "gce-proj",
				attrs: map[string]string{
					"cluster-name":     "gce-cluster",
					"cluster-location": "gce-loc",
				},
			},
			wantProjectID:      "flag-proj",
			wantCluster:        "gce-cluster",
			wantLocation:       "flag-loc",
			wantOnGCECalls:     1,
			wantProjectIDCalls: 0,
			wantAttrCalls:      1,
		},
		{
			name:      "not on GCE leaves empty flags unset",
			projectID: "",
			cluster:   "",
			location:  "",
			md: &fakeMetadataProvider{
				onGCE:     false,
				projectID: "gce-proj",
			},
			wantProjectID:      "",
			wantCluster:        "",
			wantLocation:       "",
			wantOnGCECalls:     1,
			wantProjectIDCalls: 0,
			wantAttrCalls:      0,
		},
		{
			name:      "metadata errors are returned and successful fields populated",
			projectID: "",
			cluster:   "",
			location:  "",
			md: &fakeMetadataProvider{
				onGCE:        true,
				projectIDErr: errors.New("project-id error"),
				attrs: map[string]string{
					"cluster-name": "gce-cluster",
				},
				attrErrs: map[string]error{
					"cluster-location": errors.New("location error"),
				},
			},
			wantProjectID:      "",
			wantCluster:        "gce-cluster",
			wantLocation:       "",
			wantOnGCECalls:     1,
			wantProjectIDCalls: 1,
			wantAttrCalls:      2,
			wantErr:            true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectID := tc.projectID
			cluster := tc.cluster
			location := tc.location

			err := populateMetadataDefaults(context.Background(), tc.md, &projectID, &cluster, &location)
			if (err != nil) != tc.wantErr {
				t.Fatalf("populateMetadataDefaults() error = %v, wantErr %v", err, tc.wantErr)
			}
			if projectID != tc.wantProjectID {
				t.Errorf("projectID = %q, want %q", projectID, tc.wantProjectID)
			}
			if cluster != tc.wantCluster {
				t.Errorf("cluster = %q, want %q", cluster, tc.wantCluster)
			}
			if location != tc.wantLocation {
				t.Errorf("location = %q, want %q", location, tc.wantLocation)
			}
			if tc.md.onGCECalls != tc.wantOnGCECalls {
				t.Errorf("onGCECalls = %d, want %d", tc.md.onGCECalls, tc.wantOnGCECalls)
			}
			if tc.md.projectIDCalls != tc.wantProjectIDCalls {
				t.Errorf("projectIDCalls = %d, want %d", tc.md.projectIDCalls, tc.wantProjectIDCalls)
			}
			if len(tc.md.attrCalls) != tc.wantAttrCalls {
				t.Errorf("len(attrCalls) = %d (%v), want %d", len(tc.md.attrCalls), tc.md.attrCalls, tc.wantAttrCalls)
			}
			if tc.wantOnGCECalls > 0 && !tc.md.observedTimeout {
				t.Error("expected metadata calls to use a context with timeout")
			}
		})
	}
}

func TestWarnDeprecatedWebhookFlags(t *testing.T) {
	newFlagSet := func(args []string) *flag.FlagSet {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		_ = fs.String("project-id", "", "")
		_ = fs.String("tls-cert-base64", "", "")
		_ = fs.String("tls-key-base64", "", "")
		_ = fs.String("ca-cert-base64", "", "")
		_ = fs.String("cert-dir", defaultTLSDir, "")
		_ = fs.String("webhook-addr", ":10250", "")
		if err := fs.Parse(args); err != nil {
			t.Fatalf("parse flags: %v", err)
		}
		return fs
	}

	t.Run("no deprecated flags logs nothing", func(t *testing.T) {
		var logs []string
		logger := funcr.New(func(_, args string) {
			logs = append(logs, args)
		}, funcr.Options{})

		warnDeprecatedWebhookFlags(newFlagSet([]string{"-project-id=test-proj"}), logger)
		if len(logs) != 0 {
			t.Fatalf("expected no warning logs, got %v", logs)
		}
	})

	t.Run("deprecated flags log one warning without flag values", func(t *testing.T) {
		var logs []string
		logger := funcr.New(func(_, args string) {
			logs = append(logs, args)
		}, funcr.Options{})

		warnDeprecatedWebhookFlags(newFlagSet([]string{
			"-project-id=test-proj",
			"-webhook-addr=:10250",
			"-tls-key-base64=super-secret-key",
			"-ca-cert-base64=ca-data",
		}), logger)

		if len(logs) != 1 {
			t.Fatalf("expected 1 warning log, got %d: %v", len(logs), logs)
		}
		for _, want := range []string{"deprecated webhook flags are ignored", "-ca-cert-base64", "-tls-key-base64", "-webhook-addr"} {
			if !strings.Contains(logs[0], want) {
				t.Errorf("log %q does not contain %q", logs[0], want)
			}
		}
		if strings.Contains(logs[0], "super-secret-key") || strings.Contains(logs[0], "ca-data") {
			t.Errorf("log %q must not contain flag values", logs[0])
		}
	})
}
