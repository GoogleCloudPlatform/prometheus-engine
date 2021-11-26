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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"

	v1 "k8s.io/api/certificates/v1"
	v1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/certificate/csr"
	"k8s.io/client-go/util/keyutil"
)

// provisionCSR generates a Kubernetes Certificate Signing Request using the
// fully-qualified domain name (FQDN), and randomly-generated RSA key pair.
// It returns the CSR name, the PEM-encoded private key bytes,
// and any potential errors encountered.
func provisionCSR(client kubernetes.Interface, fqdn string) (string, []byte, error) {
	var (
		template = &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   fqdn,
				Organization: []string{"system:nodes"},
			},
			DNSNames: []string{fqdn},
		}
		signerName = v1.KubeletServingSignerName
		usages     = []v1.KeyUsage{
			v1.UsageDigitalSignature,
			v1.UsageKeyEncipherment,
			v1.UsageServerAuth,
		}
		keyBuffer = bytes.Buffer{}
	)

	// Generate private/public key pair, CSR, and submit to kube apiserver.
	keyPair, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, err
	}
	csrBytes, err := cert.MakeCSRFromTemplate(keyPair, template)
	if err != nil {
		return "", nil, err
	}
	name, _, err := csr.RequestCertificate(client, csrBytes, fqdn, signerName, nil, usages, keyPair)
	if err != nil {
		return name, nil, err
	}
	err = pem.Encode(&keyBuffer, &pem.Block{
		Type:  keyutil.RSAPrivateKeyBlockType,
		Bytes: x509.MarshalPKCS1PrivateKey(keyPair),
	})
	if err != nil {
		return name, nil, err
	}
	return name, keyBuffer.Bytes(), nil
}

// deleteOldCSR removes any leftover CSR state from previous operations.
func deleteOldCSR(ctx context.Context, client kubernetes.Interface, name string) error {
	var (
		apiV1 = client.CertificatesV1().CertificateSigningRequests()
	)
	if err := apiV1.Delete(ctx, name, metav1.DeleteOptions{}); !apierrors.IsNotFound(err) {
		return err
	}
	// Error was API not found, try v1b1 API.
	var apiV1b1 = client.CertificatesV1beta1().CertificateSigningRequests()
	if err := apiV1b1.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// approveCSR fetches an existing Certificate Signing Request by name and approves it.
// It returns the updated CSR and any encountered errors.
func approveCSR(ctx context.Context, client kubernetes.Interface, name string) ([]byte, error) {
	// Try using v1 API.
	var (
		apiV1    = client.CertificatesV1().CertificateSigningRequests()
		certName string
		reqUID   types.UID
	)

	// Try both v1 and v1b1 API to approve the CSR.
	if req, err := apiV1.Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		// Error was API not found, try v1b1 API.
		var apiV1b1 = client.CertificatesV1beta1().CertificateSigningRequests()
		if req, err := apiV1b1.Get(ctx, name, metav1.GetOptions{}); err != nil {
			return nil, err
		} else {
			req.Status.Conditions = append(req.Status.Conditions,
				v1beta1.CertificateSigningRequestCondition{
					Type: v1beta1.CertificateApproved,
				})
			req, err := apiV1b1.UpdateApproval(ctx, req, metav1.UpdateOptions{})
			if err != nil {
				return nil, err
			}
			certName = req.Name
			reqUID = req.UID
		}
	} else if err == nil {
		// No error means v1 API is supported.
		req.Status.Conditions = append(req.Status.Conditions,
			v1.CertificateSigningRequestCondition{
				Type:   v1.CertificateApproved,
				Status: corev1.ConditionTrue,
			})
		req, err := apiV1.UpdateApproval(ctx, name, req, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		certName = req.Name
		reqUID = req.UID
	} else if err != nil {
		// Exit with non-nil error for non-API errors.
		return nil, err
	}

	// Wait for kube apiserver to asynchronously issue certificate.
	certBytes, err := csr.WaitForCertificate(ctx, client, certName, reqUID)
	if err != nil {
		return nil, err
	}
	return certBytes, nil
}

// CreateSignedKeyPair provisions and returns a kube-apiserver-signed certificate,
// PEM-encoded private RSA key, and any encountered errors.
func CreateSignedKeyPair(ctx context.Context, client kubernetes.Interface, fqdn string) ([]byte, []byte, error) {
	err := deleteOldCSR(ctx, client, fqdn)
	if err != nil {
		return nil, nil, err
	}
	csrName, key, err := provisionCSR(client, fqdn)
	if err != nil {
		return nil, nil, err
	}
	cert, err := approveCSR(ctx, client, csrName)
	if err != nil {
		return key, nil, err
	}
	return cert, key, nil
}
