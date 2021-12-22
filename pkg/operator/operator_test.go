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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	v1 "k8s.io/api/certificates/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
)

// Mock asynchronous kube-apiserver.
func mockKubeAPIServer(ctx context.Context, client *fake.Clientset, t *testing.T) {
	var (
		csr       *v1.CertificateSigningRequest
		timeout   = 3 * time.Second
		fqdn      = fmt.Sprintf("system:node:%s.%s.svc", NameOperator, "test-ns")
		certBytes = []byte{1, 2, 3, 4}
	)
	apiV1 := client.CertificatesV1().CertificateSigningRequests()
	// Block until kube client has newly-created CSR.
	if err := wait.Poll(50*time.Millisecond, timeout, func() (bool, error) {
		var gerr error
		// Ignore any errors coming from get API call as
		csr, gerr = apiV1.Get(ctx, fqdn, metav1.GetOptions{})
		if apierrors.IsNotFound(gerr) {
			return false, nil
		}
		// Check if certificate has been approved by the signing function.
		if len(csr.Status.Conditions) < 1 {
			return false, nil
		} else {
			return csr.Status.Conditions[0].Type == v1.CertificateApproved, nil
		}
	}); err != nil {
		t.Errorf("timeout waiting for CSR: %s", err)
	}

	// Mock the kube-apiserver issuing the certificate by writing toy data to status.
	// This will unblock the keypair generation goroutine.
	csr.Status.Certificate = certBytes
	if _, err := apiV1.UpdateStatus(ctx, csr, metav1.UpdateOptions{}); err != nil {
		t.Errorf("updating csr while issuing cert: %s", err)
	}
}

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
			opts:       Options{Key: "a2V5", Cert: "Y2VydA==", OperatorNamespace: "test-ns"},
			expectCert: "cert",
			expectKey:  "key",
			expectErr:  false,
		},
		{
			desc:      "bad cert",
			opts:      Options{Cert: "not a cert", Key: "not a key", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
		{
			desc:      "cert and no key",
			opts:      Options{Cert: "cert", Key: "", OperatorNamespace: "test-ns"},
			expectErr: true,
		},
		{
			desc:      "no cert and key",
			opts:      Options{Cert: "", Key: "key", OperatorNamespace: "test-ns"},
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
				t.Errorf("want file cert %v; got %v", tc.opts.Cert, string(outCert))
			}
			if string(outKey) != tc.expectKey {
				t.Errorf("want key %v; got %v", tc.opts.Key, string(outKey))
			}
		})
	}
}

func TestEnsureCertsServerSigned(t *testing.T) {
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

	client := fake.NewSimpleClientset()
	go mockKubeAPIServer(ctx, client, t)

	for _, tc := range []struct {
		desc       string
		opts       Options
		expectCert []byte
		expectErr  bool
	}{
		{
			desc:       "self generate keys/cert",
			opts:       Options{Cert: "", Key: "", OperatorNamespace: "test-ns"},
			expectCert: []byte{1, 2, 3, 4},
			expectErr:  false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			op := Operator{
				opts:       tc.opts,
				kubeClient: client,
			}
			cert, err := op.ensureCerts(ctx, dir)
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("want err: %v; got %v", tc.expectErr, err)
				}
				return
			}
			// Test outputed files.
			outCert, outKey := readKeyAndCertFiles(dir, t)
			if string(outCert) != string(cert) {
				t.Errorf("want ensureCerts cert %v; got %v", tc.expectCert, outCert)
			} else if string(outCert) != string(tc.expectCert) {
				t.Errorf("want file cert %v; got %v", tc.expectCert, outCert)
			}
			// Key will be randomly generated, check if it exisits.
			if len(outKey) < 1 {
				t.Errorf("invalid private key generated: priv: %v", outKey)
			}
		})
	}
}
