// Copyright 2022 Google LLC
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

package operator

import (
	"context"
	"os"
	"path"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func readKeyAndCertFiles(dir string, t *testing.T) ([]byte, []byte) {
	outCert, err := os.ReadFile(path.Join(dir, "tls.crt"))
	if err != nil {
		t.Fatalf("error reading from cert file: %v", err)
	}
	outKey, err := os.ReadFile(path.Join(dir, "tls.key"))
	if err != nil {
		t.Fatalf("error reading from key file: %v", err)
	}
	return outCert, outKey
}

func TestEnsureCertsExplicit(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_ensure_certs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	for _, tc := range []struct {
		desc         string
		opts         Options
		expectCert   string
		expectKey    string
		expectCaCert string
		expectErr    bool
	}{
		{
			desc:         "input key/cert/ca",
			opts:         Options{TLSKey: "a2V5", TLSCert: "Y2VydA==", CACert: "Y2FjZXJ0", OperatorNamespace: "test-ns"},
			expectCert:   "cert",
			expectKey:    "key",
			expectCaCert: "cacert",
			expectErr:    false,
		},
		{
			desc:       "cert/key and no CA",
			opts:       Options{TLSKey: "a2V5", TLSCert: "Y2VydA==", OperatorNamespace: "test-ns"},
			expectCert: "cert",
			expectKey:  "key",
			expectErr:  false,
		},
		{
			desc:      "bad cert",
			opts:      Options{TLSCert: "not a cert", TLSKey: "not a key", CACert: "not a CA", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
		{
			desc:      "cert and no key/ca",
			opts:      Options{TLSCert: "cert", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
		{
			desc:      "key and no cert/ca",
			opts:      Options{TLSKey: "key", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
		{
			desc:      "ca and no cert/key",
			opts:      Options{CACert: "CAcert", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			op := Operator{
				opts: tc.opts,
			}
			caBundle, err := op.ensureCerts(dir)
			if (err == nil && tc.expectErr) || (err != nil && !tc.expectErr) {
				t.Fatalf("want err: %v; got %v", tc.expectErr, err)
			}
			if err != nil && tc.expectErr {
				return
			}
			// Test outputed files.
			outCert, outKey := readKeyAndCertFiles(dir, t)
			if string(outCert) != tc.expectCert {
				t.Errorf("want cert: %v; got %v", tc.opts.TLSCert, string(outCert))
			}
			if string(outKey) != tc.expectKey {
				t.Errorf("want key: %v; got %v", tc.opts.TLSKey, string(outKey))
			}
			if string(caBundle) != tc.expectCaCert {
				t.Errorf("want ca: %v; got %v", string(caBundle), string(outCert))
			}
		})
	}
}

func TestEnsureCertsSelfSigned(t *testing.T) {
	dir, err := os.MkdirTemp("", "test_ensure_certs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	for _, tc := range []struct {
		desc      string
		opts      Options
		expectErr bool
	}{
		{
			desc:      "self generate keys/cert",
			opts:      Options{TLSCert: "", TLSKey: "", OperatorNamespace: "test-ns"},
			expectErr: false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			op := Operator{opts: tc.opts}

			caBundle, err := op.ensureCerts(dir)
			if (err == nil && tc.expectErr) || (err != nil && !tc.expectErr) {
				t.Fatalf("want err: %v; got %v", tc.expectErr, err)
			}
			if err != nil && tc.expectErr {
				return
			}
			// Cert and key will be randomly generated, check if they exisits.
			outCert, outKey := readKeyAndCertFiles(dir, t)
			if len(outKey) == 0 {
				t.Errorf("expected generated key but was empty")
			}
			if len(outCert) == 0 {
				t.Errorf("expected generated cert but was empty")
			}
			// self-generate case, ca is equal to crt.
			if string(outCert) != string(caBundle) {
				t.Errorf("want ca: %v; got %v", string(outCert), string(caBundle))
			}
		})
	}
}

func TestCleanupOldResources(t *testing.T) {
	var cases = []struct {
		desc             string
		cleanupAnnotKey  string
		collectorAnnots  map[string]string
		evaluatorAnnots  map[string]string
		collectorDeleted bool
		evaluatorDeleted bool
	}{
		{
			desc:            "keep both",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			collectorDeleted: false,
			evaluatorDeleted: false,
		},
		{
			desc:            "delete both",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"cleanme": "true",
			},
			collectorDeleted: true,
			evaluatorDeleted: true,
		},
		{
			desc:            "delete collector",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			collectorDeleted: true,
			evaluatorDeleted: false,
		},
		{
			desc:            "delete rule-evaluator",
			cleanupAnnotKey: "dont-cleanme",
			collectorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"cleanme": "true",
			},
			collectorDeleted: false,
			evaluatorDeleted: true,
		},
		{
			desc:            "keep both",
			cleanupAnnotKey: "",
			collectorAnnots: map[string]string{
				"dont-cleanme": "true",
			},
			evaluatorAnnots: map[string]string{
				"cleanme": "true",
			},
			collectorDeleted: false,
			evaluatorDeleted: false,
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			ctx := context.Background()
			ds := &appsv1.DaemonSet{
				ObjectMeta: v1.ObjectMeta{
					Name:        NameCollector,
					Namespace:   "gmp-system",
					Annotations: c.collectorAnnots,
				},
			}

			deploy := &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Name:        NameRuleEvaluator,
					Namespace:   "gmp-system",
					Annotations: c.evaluatorAnnots,
				},
			}
			opts := Options{
				ProjectID:         "test-proj",
				Location:          "test-loc",
				Cluster:           "test-cluster",
				OperatorNamespace: "gmp-system",
				CleanupAnnotKey:   c.cleanupAnnotKey,
			}
			cl := fake.NewClientBuilder().WithObjects(ds, deploy).Build()

			op := &Operator{
				logger: testr.New(t),
				opts:   opts,
				client: cl,
			}
			if err := op.cleanupOldResources(ctx); err != nil {
				t.Fatal(err)
			}

			// Check if collector DaemonSet was preserved.
			var gotDS appsv1.DaemonSet
			dsErr := cl.Get(ctx, client.ObjectKey{
				Name:      NameCollector,
				Namespace: "gmp-system",
			}, &gotDS)
			if c.collectorDeleted {
				if !apierrors.IsNotFound(dsErr) {
					t.Errorf("collector should be deleted but found: %+v", gotDS)
				}
			} else if gotDS.Name != ds.Name || gotDS.Namespace != ds.Namespace {
				t.Errorf("collector DaemonSet differs")
			}

			// Check if rule-evaluator Deployment was preserved.
			var gotDeploy appsv1.Deployment
			deployErr := cl.Get(ctx, client.ObjectKey{
				Name:      NameRuleEvaluator,
				Namespace: "gmp-system",
			}, &gotDeploy)
			if c.evaluatorDeleted {
				if !apierrors.IsNotFound(deployErr) {
					t.Errorf("rule-evaluator should be deleted but found: %+v", gotDeploy)
				}
			} else if gotDeploy.Name != deploy.Name || gotDeploy.Namespace != deploy.Namespace {
				t.Errorf("rule-evaluator Deployment differs")
			}
		})
	}
}
