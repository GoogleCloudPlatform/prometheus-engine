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

package v1

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/prometheus/common/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/secrets"
)

// SecretSelector represents a reference to a secret from the given provider
// e.g. Kubernetes Secret. Only one provider can be used at the same time.
type SecretSelector struct {
	// KubernetesSecret represents reference to a given key from certain Kubernetes Secret
	// in a given namespace.
	// +optional
	KubernetesSecret *KubernetesSecretKeySelector `json:"kubernetesSecret,omitempty"`
}

func (s *SecretSelector) toPrometheusSecretRef(m PodMonitoringCRD, pool PrometheusSecretConfigs) (string, error) {
	if s == nil {
		return "", nil
	}

	if s.KubernetesSecret == nil {
		return "", nil
	}

	return s.KubernetesSecret.toPrometheusSecretRef(m, pool)
}

// KubernetesSecretKeySelector represents selector for Kubernetes secret.
// It's similar to k8s.io/api/core/v1.SecretKeySelector, but
// allows cross namespace selections.
type KubernetesSecretKeySelector struct {
	// Name of the secret to select from.
	Name string `json:"name"`

	// The key of the secret to select from. Must be a valid secret key.
	Key string `json:"key"`

	// Namespace of the secret to select from.
	// If empty the parent resource namespace will be chosen.
	// +optional
	// +kubebuilder:field:scope=Cluster
	Namespace string `json:"namespace,omitempty"`
}

// PrometheusSecretConfigs allows quick gathering of SecretConfigs for Prometheus configuration.
// NOTE(bwplotka): This could be removed depending on how upstream Prometheus would like us to reference the secrets.
type PrometheusSecretConfigs map[string]secrets.KubernetesSecretConfig

// Set inserts kubernetes.SecretConfig for given reference (in form of <namespace>/<name>/<key>).
// Insertion will be deduplication if needed.
func (p PrometheusSecretConfigs) Set(ref string, c secrets.KubernetesSecretConfig) {
	if p == nil {
		return
	}
	if _, ok := p[ref]; ok {
		return
	}
	p[ref] = c
}

// SecretConfigs returns an unordered list of secrets.SecretConfig.
func (p PrometheusSecretConfigs) SecretConfigs() []secrets.SecretConfig {
	ret := make([]secrets.SecretConfig, 0, len(p))
	for ref, c := range p {
		ret = append(ret, secrets.SecretConfig{
			Name:   ref,
			Config: c,
		})
	}
	return ret
}

// toPrometheusSecretRef returns Prometheus reference to Kubernetes secret (or empty string if not set).
// It also adds secret to list of secrets by argument reference, if exists.
// It returns error if namespace tenancy security restriction is violated for PodMonitoring.
func (s *KubernetesSecretKeySelector) toPrometheusSecretRef(m PodMonitoringCRD, pool PrometheusSecretConfigs) (string, error) {
	if s == nil {
		return "", nil
	}

	ns := s.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}
	if m.IsNamespaceScoped() {
		monitoringNamespace := m.GetNamespace()
		if monitoringNamespace == "" {
			monitoringNamespace = metav1.NamespaceDefault
		}
		if ns != monitoringNamespace {
			return "", fmt.Errorf("PodMonitoring secret selector can't select secret from the different namespace than PodMonitoring namespace %q, got: %q; Consider using ClusterPodMonitoring or copy secret to PodMonitoring namespace", m.GetNamespace(), ns)
		}
	}

	ref := fmt.Sprintf("%s/%s/%s", ns, s.Name, s.Key)
	pool.Set(ref, secrets.KubernetesSecretConfig{
		Namespace: ns,
		Name:      s.Name,
		Key:       s.Key,
	})
	return ref, nil
}

// Auth sets the `Authorization` header on every HTTP request.
type Auth struct {
	// The authentication type. Defaults to Bearer.
	// Basic will cause an error, as the BasicAuth object should be used instead.
	Type string `json:"type,omitempty"`
	// Credentials references the Kubernetes secret's key with the credentials
	// (token) for the auth header to send along the request.
	// Optional, as in previous resource versions we allowed no credentials.
	// +optional
	Credentials *SecretSelector `json:"credentials,omitempty"`
}

func (c *Auth) ToPrometheusConfig(m PodMonitoringCRD, pool PrometheusSecretConfigs) (*config.Authorization, error) {
	ref, err := c.Credentials.toPrometheusSecretRef(m, pool)
	if err != nil {
		return nil, err
	}
	return &config.Authorization{
		Type:           c.Type,
		CredentialsRef: ref,
	}, nil
}

// BasicAuth sets the `Authorization` header on every HTTP request with the
// configured username and optional password.
type BasicAuth struct {
	// The username for authentication.
	Username string `json:"username,omitempty"`
	// Password references the Kubernetes secret's key with the password to use.
	// Optional, as in previous resource versions we allowed no credentials.
	// +optional
	Password *SecretSelector `json:"password,omitempty"`
}

func (c *BasicAuth) ToPrometheusConfig(m PodMonitoringCRD, pool PrometheusSecretConfigs) (*config.BasicAuth, error) {
	ref, err := c.Password.toPrometheusSecretRef(m, pool)
	if err != nil {
		return nil, err
	}
	return &config.BasicAuth{
		Username:    c.Username,
		PasswordRef: ref,
	}, nil
}

// TLS specifies TLS configuration used for HTTP requests.
type TLS struct {
	// Used to verify the hostname for the targets.
	ServerName string `json:"serverName,omitempty"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	MinVersion string `json:"minVersion,omitempty"`
	// Maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	MaxVersion string `json:"maxVersion,omitempty"`

	// CA references the Kubernetes secret's key with the CA certificate to
	// validate API server certificate with.
	// Optional, as in previous resource versions we allowed no credentials.
	// +optional
	CA *SecretSelector `json:"ca,omitempty"`
	// Cert references the Kubernetes secret's key with the certificate (public
	// key) for client cert authentication to the server.
	// Optional, as in previous resource versions we allowed no credentials.
	// +optional
	Cert *SecretSelector `json:"cert,omitempty"`
	// Key references the Kubernetes secret's key with the private key
	// for client cert authentication to the server.
	// Optional, as in previous resource versions we allowed no credentials.
	// +optional
	Key *SecretSelector `json:"key,omitempty"`
}

func TLSVersionFromString(s string) (config.TLSVersion, error) {
	if s == "" {
		return 0, nil
	}
	if v, ok := config.TLSVersions[s]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unknown TLS version: %s", s)
}

func (c *TLS) ToPrometheusConfig(m PodMonitoringCRD, pool PrometheusSecretConfigs) (*config.TLSConfig, error) {
	tls := &config.TLSConfig{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
	}

	var err error
	var errs []error
	tls.MinVersion, err = TLSVersionFromString(c.MinVersion)
	if err != nil {
		errs = append(errs, fmt.Errorf("unable to convert TLS min version: %w", err))
	}
	tls.MaxVersion, err = TLSVersionFromString(c.MaxVersion)
	if err != nil {
		errs = append(errs, fmt.Errorf("unable to convert TLS max version: %w", err))
	}
	tls.CARef, err = c.CA.toPrometheusSecretRef(m, pool)
	if err != nil {
		errs = append(errs, err)
	}
	tls.CertRef, err = c.Cert.toPrometheusSecretRef(m, pool)
	if err != nil {
		errs = append(errs, err)
	}
	tls.KeyRef, err = c.Key.toPrometheusSecretRef(m, pool)
	if err != nil {
		errs = append(errs, err)
	}
	return tls, errors.Join(errs...)
}

// OAuth2 is the OAuth2 client configuration.
type OAuth2 struct {
	// Public identifier for the client.
	ClientID string `json:"clientID"`
	// ClientSecret references the Kubernetes secret's key with the client secret
	// token for Oauth2 flow.
	// Optional, as in previous resource versions we allowed no credentials.
	// +optional
	ClientSecret *SecretSelector `json:"clientSecret,omitempty"`
	// Scopes for the token request.
	Scopes []string `json:"scopes,omitempty"`
	// The URL to fetch the token from.
	TokenURL string `json:"tokenURL"`
	// Optional parameters to append to the token URL.
	EndpointParams map[string]string `json:"endpointParams,omitempty"`
	// Configures the token request's TLS settings.
	TLS         *TLS `json:"tlsConfig,omitempty"`
	ProxyConfig `json:",inline"`
}

func (c *OAuth2) ToPrometheusConfig(m PodMonitoringCRD, pool PrometheusSecretConfigs) (*config.OAuth2, error) {
	oauth2 := &config.OAuth2{
		ClientID:       c.ClientID,
		Scopes:         c.Scopes,
		TokenURL:       c.TokenURL,
		EndpointParams: c.EndpointParams,
	}
	var errs []error
	clientSecret, err := c.ClientSecret.toPrometheusSecretRef(m, pool)
	if err != nil {
		errs = append(errs, err)
	} else {
		oauth2.ClientSecretRef = clientSecret
	}
	if c.TLS != nil {
		tlsConfig, err := c.TLS.ToPrometheusConfig(m, pool)
		if err != nil {
			errs = append(errs, fmt.Errorf("OAuth2 TLS: %w", err))
		} else {
			oauth2.TLSConfig = *tlsConfig
		}
	}
	if c.ProxyConfig.ProxyURL != "" {
		proxyConfig, err := c.ProxyConfig.ToPrometheusConfig()
		if err != nil {
			errs = append(errs, fmt.Errorf("OAuth2 proxy config: %w", err))
		} else {
			oauth2.ProxyURL = proxyConfig
		}
	}
	return oauth2, errors.Join(errs...)
}

type ProxyConfig struct {
	// HTTP proxy server to use to connect to the targets. Encoded passwords are not supported.
	ProxyURL string `json:"proxyUrl,omitempty"`

	// TODO(TheSpiritXIII): Consider adding further fields for Proxy configuration, similar to https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2
}

func (c *ProxyConfig) ToPrometheusConfig() (config.URL, error) {
	proxyURL, err := url.Parse(c.ProxyURL)
	if err != nil {
		return config.URL{}, fmt.Errorf("invalid proxy URL: %w", err)
	}
	// Marshalling the config will redact the password, so we don't support those.
	// It's not a good idea anyway, and we could add later support basic auth based on secrets to
	// cover the general use case.
	if _, ok := proxyURL.User.Password(); ok {
		return config.URL{}, errors.New("passwords encoded in URLs are not supported")
	}
	// Initialize from default as encode/decode does not work correctly with the type definition.
	return config.URL{URL: proxyURL}, nil
}

// HTTPClientConfig stores HTTP-client configurations.
type HTTPClientConfig struct {
	// The HTTP authorization credentials for the targets.
	Authorization *Auth `json:"authorization,omitempty"`
	// The HTTP basic authentication credentials for the targets.
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
	// Configures the scrape request's TLS settings.
	TLS *TLS `json:"tls,omitempty"`
	// The OAuth2 client credentials used to fetch a token for the targets.
	OAuth2 *OAuth2 `json:"oauth2,omitempty"`
	// Proxy configuration.
	ProxyConfig `json:",inline"`
}

func (c *HTTPClientConfig) ToPrometheusConfig(m PodMonitoringCRD, pool PrometheusSecretConfigs) (config.HTTPClientConfig, error) {
	// Copy default config.
	clientConfig := config.DefaultHTTPClientConfig

	var errs []error
	if c.Authorization != nil {
		var err error
		clientConfig.Authorization, err = c.Authorization.ToPrometheusConfig(m, pool)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if c.BasicAuth != nil {
		var err error
		clientConfig.BasicAuth, err = c.BasicAuth.ToPrometheusConfig(m, pool)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if c.TLS != nil {
		tlsConfig, err := c.TLS.ToPrometheusConfig(m, pool)
		if err != nil {
			errs = append(errs, err)
		} else {
			clientConfig.TLSConfig = *tlsConfig
		}
	}
	if c.OAuth2 != nil {
		oauth2, err := c.OAuth2.ToPrometheusConfig(m, pool)
		if err != nil {
			errs = append(errs, err)
		} else {
			clientConfig.OAuth2 = oauth2
		}
	}
	if c.ProxyConfig.ProxyURL != "" {
		proxyConfig, err := c.ProxyConfig.ToPrometheusConfig()
		if err != nil {
			errs = append(errs, err)
		} else {
			clientConfig.ProxyURL = proxyConfig
		}
	}
	return clientConfig, errors.Join(errs...)
}
