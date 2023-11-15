// Copyright 2024 Google LLC
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

// Package certupdater contains an implementation of `tls.GetCertificate`.
package certupdater

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/cert"
)

const tlsCAKey = "ca.crt"

type certUpdater struct {
	mu sync.RWMutex

	logger logr.Logger

	source      CertSource
	currentCert *tls.Certificate
	currentCA   *x509.Certificate

	pollingInterval time.Duration
}

// Option configures a new certUpdater.
type Option func(*certUpdater)

// WithLogging provides a logger to certUpdater.
func WithLogging(l logr.Logger) Option {
	return func(cu *certUpdater) {
		cu.logger = l.WithValues("package", "certupdater")
	}
}

// WithPolling causes certUpdater to check for changes to certificates periodically.
func WithPolling(d time.Duration) Option {
	return func(cu *certUpdater) {
		cu.pollingInterval = d
	}
}

// New creates a new certUpdater.
//
//nolint:revive // Intentionally return unexported type, to use methods only.
func New(source CertSource, opts ...Option) (*certUpdater, error) {
	if source == nil {
		return nil, fmt.Errorf("source must not be nil")
	}
	cu := &certUpdater{
		source: source,
	}
	for _, opt := range opts {
		opt(cu)
	}
	return cu, nil
}

// Start begins polling to periodically update certificates from the available sources.
func (cu *certUpdater) Start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-time.After(cu.pollingInterval):
				if err := cu.poll(ctx); err != nil {
					cu.logger.Error(err, "Updating certs failed.")
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (cu *certUpdater) poll(ctx context.Context) error {
	cert, ca, err := cu.source(ctx)
	if err != nil {
		return fmt.Errorf("cert source: %w", err)
	}
	cu.mu.Lock()
	cu.currentCert = &cert
	cu.currentCA = ca
	cu.mu.Unlock()

	return nil
}

// GetCA allows access to CA Bundle, if applicable.
func (cu *certUpdater) GetCA() (*x509.Certificate, error) {
	cu.mu.RLock()
	defer cu.mu.RUnlock()
	return cu.currentCA, nil
}

// GetCertificate implements tls.Config.GetCertificate. Certificates are updated asynchronously.
func (cu *certUpdater) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cu.mu.RLock()
	defer cu.mu.RUnlock()
	return cu.currentCert, nil
}

// CertSource defines the common signature of functions that return certificates.
type CertSource func(ctx context.Context) (webhook tls.Certificate, ca *x509.Certificate, err error)

// SourceBase64 sources certificates from base64 strings.
func SourceBase64(certString, keyString string, optionalCAString string) (CertSource, error) {
	cert, err := base64.StdEncoding.DecodeString(certString)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(keyString)
	if err != nil {
		return nil, err
	}

	// Return nil CA, if not provided
	if optionalCAString == "" {
		return sourcePEM(cert, key, nil)
	}

	ca, err := base64.StdEncoding.DecodeString(optionalCAString)
	if err != nil {
		return nil, err
	}
	return sourcePEM(cert, key, ca)
}

// SourceDir sources certificates from directory on the host.
//
// Expected Certificate Name: `tls.crt`
// Expected Private Key Name: `tls.key`
// Expected CA Certificate Name: `ca.crt` [Optional]
//
// Invalid directories or missing files will result in an error.
func SourceDir(dir string) (CertSource, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	return SourceFS(os.DirFS(dir).(fs.ReadFileFS))
}

// SourceFS sources certificates from an `io/fs.FS` abstraction
// Expected Certificate Name: `tls.crt`
// Expected Private Key Name: `tls.key`
// Expected CA Certificate Name: `ca.crt` [Optional]
//
// If the CA Certificate is not provided, the `tls.crt` will be used as its own CA.
func SourceFS(fsys fs.ReadFileFS) (CertSource, error) {
	// Perform all checks to fail fast if certs do not exist or are invalid when creating the source
	certPEM, err := fsys.ReadFile(corev1.TLSCertKey)
	if err != nil {
		return nil, err
	}
	keyPEM, err := fsys.ReadFile(corev1.TLSPrivateKeyKey)
	if err != nil {
		return nil, err
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, err
	}

	if caPEM, err := fsys.ReadFile(tlsCAKey); err == nil {
		if _, err := x509.ParseCertificate(pemToDER(caPEM)); err != nil {
			return nil, err
		}
	}

	return func(context.Context) (tls.Certificate, *x509.Certificate, error) {
		certPEM, err := fsys.ReadFile(corev1.TLSCertKey)
		if err != nil {
			return tls.Certificate{}, nil, err
		}
		keyPEM, err := fsys.ReadFile(corev1.TLSPrivateKeyKey)
		if err != nil {
			return tls.Certificate{}, nil, err
		}
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return tls.Certificate{}, nil, err
		}

		caPEM, err := fsys.ReadFile(tlsCAKey)
		if err != nil {
			//nolint:nilerr // Return nil for CA if it is missing. Not an error.
			return cert, nil, nil
		}

		ca, err := x509.ParseCertificate(pemToDER(caPEM))
		if err != nil {
			return tls.Certificate{}, nil, err
		}

		return cert, ca, nil
	}, nil
}

// SourceGenerated generates self-signed certificates.
func SourceGenerated(fqdn string) (CertSource, error) {
	crt, key, err := cert.GenerateSelfSignedCertKey(fqdn, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("generate self-signed TLS key pair: %w", err)
	}
	return sourcePEM(crt, key, crt)
}

// sourcePEM sources certificates from a PEM-formatted input.
func sourcePEM(certPEM, keyPEM, optionalCAPEM []byte) (CertSource, error) {
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	var ca *x509.Certificate
	if len(optionalCAPEM) > 0 {
		ca, err = x509.ParseCertificate(pemToDER(optionalCAPEM))
		if err != nil {
			return nil, err
		}
	}

	return func(context.Context) (tls.Certificate, *x509.Certificate, error) {
		return certificate, ca, nil
	}, nil
}

func pemToDER(in []byte) []byte {
	p, _ := pem.Decode(in)
	return p.Bytes
}
