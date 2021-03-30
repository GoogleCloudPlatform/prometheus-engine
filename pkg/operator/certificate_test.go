package operator

import (
	"bytes"
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetKubeTLSKeyPair(t *testing.T) {
	var (
		timeout     = 3 * time.Second
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		client      = fake.NewSimpleClientset()
		apiV1       = client.CertificatesV1().CertificateSigningRequests()
		csr         *v1.CertificateSigningRequest
		certBytes   = []byte{1, 2, 3, 4}
		fqdn        = "test-svc.test-ns.svc"
	)
	t.Cleanup(cancel)

	// Fire off a goroutine to mock asynchronous kube-apiserver.
	go func() {
		// Block until kube client has newly-created CSR.
		if err := wait.Poll(50*time.Millisecond, timeout, func() (bool, error) {
			var gerr error
			// Ignore any errors coming from get API call as
			csr, gerr = apiV1.Get(ctx, fqdn, metav1.GetOptions{})
			if gerr != nil {
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
}
