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
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"
)

func readKeyAndCertFiles(dir string, t *testing.T) ([]byte, []byte) {
	outCert, err := ioutil.ReadFile(path.Join(dir, "tls.crt"))
	if err != nil {
		t.Fatalf("error reading from cert file: %v", err)
	}
	outKey, err := ioutil.ReadFile(path.Join(dir, "tls.key"))
	if err != nil {
		t.Fatalf("error reading from key file: %v", err)
	}
	return outCert, outKey
}

func TestEnsureCertsExplicit(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_ensure_certs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	for _, tc := range []struct {
		desc       string
		opts       Options
		expectCert string
		expectKey  string
		expectErr  bool
	}{
		{
			desc:       "input key/cert",
			opts:       Options{TLSKey: "a2V5", TLSCert: "Y2VydA==", OperatorNamespace: "test-ns"},
			expectCert: "cert",
			expectKey:  "key",
			expectErr:  false,
		},
		{
			desc:      "bad cert",
			opts:      Options{TLSCert: "not a cert", TLSKey: "not a key", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
		{
			desc:      "cert and no key",
			opts:      Options{TLSCert: "cert", TLSKey: "", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
		{
			desc:      "no cert and key",
			opts:      Options{TLSCert: "", TLSKey: "key", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			op := Operator{
				opts: tc.opts,
			}
			cert, err := op.ensureCerts(context.Background(), dir)
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("want err: %v; got %v", tc.expectErr, err)
				}
				return
			}
			// Test outputed files.
			outCert, outKey := readKeyAndCertFiles(dir, t)
			if string(outCert) != string(cert) {
				t.Errorf("want ensureCerts cert %v; got %v", string(cert), string(outCert))
			} else if string(outCert) != tc.expectCert {
				t.Errorf("want file cert %v; got %v", tc.opts.TLSCert, string(outCert))
			}
			if string(outKey) != tc.expectKey {
				t.Errorf("want key %v; got %v", tc.opts.TLSKey, string(outKey))
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

	dir, err := ioutil.TempDir("", "test_ensure_certs")
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

			_, err := op.ensureCerts(ctx, dir)
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("want err: %v; got %v", tc.expectErr, err)
				}
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
		})
	}
}
