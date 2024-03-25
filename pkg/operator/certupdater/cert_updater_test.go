// Copyright 2023 Google LLC
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

// Package operator contains the Prometheus operator.
package certupdater

import (
	"context"
	"crypto/x509"
	"embed"
	_ "embed"
	"io/fs"
	"testing"
)

//go:embed testdata/valid/tls.crt.b64
var testCertB64 string

//go:embed testdata/valid/tls.key.b64
var testKeyB64 string

//go:embed testdata/valid/tls.crt.b64
var testCAB64 string

//go:embed testdata/valid/tls.crt
var testCertBytes []byte

//go:embed testdata/valid/tls.key
var testKeyBytes []byte

//go:embed testdata/valid/tls.crt
var testCABytes []byte

//go:embed testdata/valid/*
var testFsys embed.FS

func TestGetCertificate(t *testing.T) {
	source, err := sourcePEM(testCertBytes, testKeyBytes, nil)
	if err != nil {
		t.Error(err)
	}
	cu, err := New(source)
	if err != nil {
		t.Error(err)
	}

	if err := cu.poll(context.Background()); err != nil {
		t.Error(err)
	}

	cert, err := cu.GetCertificate(nil)
	if err != nil {
		t.Error(err)
	}
	if cert.Certificate == nil {
		t.Error("Certificate is nil")
	}
	if cert.PrivateKey == nil {
		t.Error("Private key is nil")
	}
}

func TestSourceBase64(t *testing.T) {
	type test struct {
		keyString  string
		certString string
		caString   string
		wantCA     bool
		wantErr    bool
	}
	tests := map[string]test{
		"valid b64 pair": {
			keyString:  testKeyB64,
			certString: testCertB64,
			wantErr:    false,
		},
		"valid b64 pair with CA": {
			keyString:  testKeyB64,
			certString: testCertB64,
			caString:   testCAB64,
			wantCA:     true,
			wantErr:    false,
		},
		"invalid key/cert": {
			keyString:  "dGVzdAo=",
			certString: "dGVzdAo=",
			wantErr:    true,
		},
		"invalid b64 encoding of key": {
			keyString: "%",
			wantErr:   true,
		},
		"invalid b64 encoding of cert": {
			certString: "%",
			wantErr:    true,
		},
		"missing private key": {
			certString: testCertB64,
			wantErr:    true,
		},
		"missing cert": {
			certString: testCertB64,
			wantErr:    true,
		},
		"missing key and cert": {
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := SourceBase64(tc.certString, tc.keyString, tc.caString)
			switch {
			case err == nil && !tc.wantErr:
				validateCertSource(t, got, tc.wantCA)
			case err != nil && tc.wantErr:
				// OK
			case err == nil && tc.wantErr:
				t.Errorf("error wanted, but got: %s", err)
			case err != nil && !tc.wantErr:
				t.Errorf("unwanted error: %s", err)
			}
		})
	}
}

func TestSourcePEM(t *testing.T) {
	type test struct {
		key     []byte
		cert    []byte
		ca      []byte
		wantCA  bool
		wantErr bool
	}
	tests := map[string]test{
		"valid pair": {
			key:     testKeyBytes,
			cert:    testCertBytes,
			wantErr: false,
		},
		"valid pair with CA": {
			key:     testKeyBytes,
			cert:    testCertBytes,
			ca:      testCABytes,
			wantCA:  true,
			wantErr: false,
		},
		"invalid pair": {
			key:     []byte{'t', 'e', 's', 't'},
			cert:    []byte{'t', 'e', 's', 't'},
			wantErr: true,
		},
		"missing private key": {
			cert:    testCertBytes,
			wantErr: true,
		},
		"missing cert": {
			cert:    testCertBytes,
			wantErr: true,
		},
		"missing key and cert": {
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := sourcePEM(tc.cert, tc.key, tc.ca)
			switch {
			case err == nil && !tc.wantErr:
				validateCertSource(t, got, tc.wantCA)
			case err != nil && tc.wantErr:
				// OK
			case err == nil && tc.wantErr:
				t.Errorf("error wanted, but got: %s", err)
			case err != nil && !tc.wantErr:
				t.Errorf("unwanted error: %s", err)
			}
		})
	}
}

func TestSourceDir(t *testing.T) {
	type test struct {
		dir     string
		wantCA  bool
		wantErr bool
	}
	tests := map[string]test{
		"valid": {
			dir: "./testdata/valid",
		},
		"valid with CA": {
			dir:    "./testdata/valid_ca",
			wantCA: true,
		},
		"invalid dir": {
			dir:     ".",
			wantErr: true,
		},
		"non-existent dir": {
			dir:     "./fakedir",
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := SourceDir(tc.dir)
			switch {
			case err == nil && !tc.wantErr:
				validateCertSource(t, got, tc.wantCA)
			case err != nil && tc.wantErr:
				// OK
			case err == nil && tc.wantErr:
				t.Errorf("error wanted, but got: %s", err)
			case err != nil && !tc.wantErr:
				t.Errorf("unwanted error: %s", err)
			}
		})
	}
}

func TestSourceFS(t *testing.T) {
	type test struct {
		sub     string
		wantCA  bool
		wantErr bool
	}
	tests := map[string]test{
		"valid": {
			sub: "testdata/valid",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fsys, err := fs.Sub(testFsys, tc.sub)
			if err != nil {
				t.Error(err)
			}
			got, err := SourceFS(fsys.(fs.ReadFileFS))
			switch {
			case err == nil && !tc.wantErr:
				validateCertSource(t, got, tc.wantCA)
			case err != nil && tc.wantErr:
				// OK
			case err == nil && tc.wantErr:
				t.Errorf("error wanted, but got: %s", err)
			case err != nil && !tc.wantErr:
				t.Errorf("unwanted error: %s", err)
			}

			if err != nil {
				t.Error(err)
			}
		})
	}
}

func TestSourceGenerated(t *testing.T) {
	type test struct {
		fqdn            string
		wantCA          bool
		wantErr         bool
		wantErrHostname bool
	}
	tests := map[string]test{
		"valid": {
			fqdn:   "gmp-operator.gmp-system.svc",
			wantCA: true,
		},
		"empty fqdn": {
			wantCA:          true,
			wantErrHostname: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := SourceGenerated(tc.fqdn)
			switch {
			case err == nil && !tc.wantErr:
				validateCertSource(t, got, tc.wantCA)

				cert, _, err := got(context.Background())
				if err != nil {
					t.Error(err)
				}
				xCert, err := x509.ParseCertificate(cert.Certificate[0])
				if err != nil {
					t.Error(err)
				}
				if err := xCert.VerifyHostname(tc.fqdn); err != nil && !tc.wantErrHostname {
					t.Error(err)
				}
			case err != nil && tc.wantErr:
				// OK
			case err == nil && tc.wantErr:
				t.Errorf("error wanted, but got: %s", err)
			case err != nil && !tc.wantErr:
				t.Errorf("unwanted error: %s", err)
			}
			validateCertSource(t, got, tc.wantCA)
		})
	}
}

func validateCertSource(t *testing.T, c CertSource, wantCA bool) {
	t.Helper()

	cert, ca, err := c(context.Background())
	if err != nil {
		t.Error(err)
	}
	if cert.Certificate == nil {
		t.Error("Certificate is nil")
	}
	if cert.PrivateKey == nil {
		t.Error("Private key is nil")
	}

	if wantCA && ca == nil {
		t.Error("Expected CA, but none found")
	}

	if !wantCA && ca != nil {
		t.Error("Expected nil CA, but found non-nil CA")
	}
}
