// Copyright 2021 Google LLC
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
	"bytes"
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/certificates/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetKubeTLSKeyPair(t *testing.T) {
	var (
		timeout     = 3 * time.Second
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		csr         *v1.CertificateSigningRequest
		certBytes   = []byte{1, 2, 3, 4}
		fqdn        = "test-svc.test-ns.svc"
	)
	t.Cleanup(cancel)

	cases := []struct {
		doc   string
		state *v1.CertificateSigningRequest
	}{
		{
			doc: "no prior state",
		},
		{
			doc: "prior state",
			state: &v1.CertificateSigningRequest{
				TypeMeta: metav1.TypeMeta{Kind: "CertificateSigningRequest"},
				ObjectMeta: metav1.ObjectMeta{
					Name: fqdn,
				},
				Spec: v1.CertificateSigningRequestSpec{
					Request: []byte{5, 6, 7, 8},
				},
			},
		},
	}

	for _, c := range cases {
		var (
			client kubernetes.Interface
		)
		if c.state != nil {
			client = fake.NewSimpleClientset(c.state)
		} else {
			client = fake.NewSimpleClientset()
		}
		apiV1 := client.CertificatesV1().CertificateSigningRequests()

		t.Run(c.doc, func(t *testing.T) {
			// Fire off a goroutine to mock asynchronous kube-apiserver.
			go func() {
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
			}()

			// Create the CSR, method will block until certificate is issued by the goroutine above.
			// Then validate certificate contents.
			if cert, priv, err := CreateSignedKeyPair(ctx, client, fqdn); err != nil {
				t.Errorf("generating kube-signed keypair: %s", err)
			} else if eq := bytes.Compare(cert, certBytes); eq != 0 || len(priv) < 1 {
				t.Errorf("invalid private key generated: priv: %v", priv)
			}
		})
	}
}
