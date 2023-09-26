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
	"time"
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
			caBundle, err := op.ensureCerts(context.Background(), dir)
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
	var (
		timeout     = 3 * time.Second
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	)
	t.Cleanup(cancel)

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

			caBundle, err := op.ensureCerts(ctx, dir)
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
