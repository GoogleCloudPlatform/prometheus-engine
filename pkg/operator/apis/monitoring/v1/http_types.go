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

	"github.com/prometheus/prometheus/google/secrets"
)

// SecretSelector references a secret from a secret provider e.g. Kubernetes Secret. Only one
// provider can be used at a time.
type SecretSelector struct {
	// Secret represents reference to a given key from certain Secret in a given namespace.
	// +optional
	Secret *SecretKeySelector `json:"secret,omitempty"`
}

func (s *SecretSelector) toPrometheusSecretRef(m PodMonitoringCRD, pool PrometheusSecretConfigs) (string, error) {
	if s == nil {
		return "", nil
	}

	if s.Secret == nil {
		return "", nil
	}

	return s.Secret.toPrometheusSecretRef(m, pool)
}

// SecretKeySelector represents selector for Kubernetes secret.
// It's similar to k8s.io/api/core/v1.SecretKeySelector, but allows cross namespace selections.
type SecretKeySelector struct {
	// Name of the secret to select from.
	Name string `json:"name"`

	// Key of the secret to select from. Must be a valid secret key.
	Key string `json:"key"`

	// Namespace of the secret to select from.
	// If empty the parent resource namespace will be chosen.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PrometheusSecretConfigs allows quick gathering of SecretConfigs for Prometheus configuration.
type PrometheusSecretConfigs map[string]secrets.KubernetesSecretConfig

// Set inserts kubernetes.SecretConfig for given reference (in form of <namespace>/<name>/<key>).
// Insertion will be deduplicated if needed.
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

// toPrometheusSecretRef returns the Prometheus reference to Kubernetes secret and adds the
// secrets to the secret pool, returning an empty string if not set.
func (s *SecretKeySelector) toPrometheusSecretRef(m PodMonitoringCRD, pool PrometheusSecretConfigs) (string, error) {
	if s == nil {
		return "", nil
	}

	ns := s.Namespace
	if ns == "" {
		if m.IsNamespaceScoped() {
			ns = m.GetNamespace()
		} else {
			ns = metav1.NamespaceDefault
		}
	} else if m.IsNamespaceScoped() && ns != m.GetNamespace() {
		return "", fmt.Errorf("must use namespace %q, got: %q", m.GetNamespace(), ns)
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
	// Type is the authentication type. Defaults to Bearer.
	// Basic will cause an error, as the BasicAuth object should be used instead.
	// +kubebuilder:validation:XValidation:rule="self != 'Basic'",message="authorization type cannot be set to \"basic\", use \"basic_auth\" instead"
	// +optional
	Type string `json:"type,omitempty"`
	// Credentials uses the secret as the credentials (token) for the authentication header.
	// +optional
	Credentials *SecretSelector `json:"credentials,omitempty"`
}

// ToPrometheusConfig converts this object into the respective Prometheus configuration.
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

// BasicAuth sets the `Authorization` header on every HTTP request with the configured username
// and password.
type BasicAuth struct {
	// Username is the BasicAuth username.
	// +optional
	Username string `json:"username,omitempty"`
	// Password uses the secret as the BasicAuth password.
	// +optional
	Password *SecretSelector `json:"password,omitempty"`
}

// ToPrometheusConfig converts this object into the respective Prometheus configuration.
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
// +kubebuilder:validation:XValidation:rule=has(self.cert) == has(self.key),message="client cert and client key must be provided together, when either is provided"
type TLS struct {
	// ServerName is used to verify the hostname for the targets.
	// +optional
	ServerName string `json:"serverName,omitempty"`
	// InsecureSkipVerify disables target certificate validation.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// MinVersion is the minimum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1),
	// TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	//
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	// +kubebuilder:validation:Enum=TLS10;TLS11;TLS12;TLS13
	// +optional
	MinVersion string `json:"minVersion,omitempty"`
	// MaxVersion is the maximum TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS 1.1),
	// TLS12 (TLS 1.2), TLS13 (TLS 1.3).
	//
	// If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
	// See MinVersion in https://pkg.go.dev/crypto/tls#Config.
	// +kubebuilder:validation:Enum=TLS10;TLS11;TLS12;TLS13
	// +optional
	MaxVersion string `json:"maxVersion,omitempty"`

	// CA uses the secret as the CA certificate to validate the server with.
	// +optional

	CA *SecretSelector `json:"ca,omitempty"`
	// Cert uses the secret as the certificate for client authentication to the server.
	// +optional
	Cert *SecretSelector `json:"cert,omitempty"`
	// Key uses the secret as the private key for client authentication to the server.
	// +optional
	Key *SecretSelector `json:"key,omitempty"`
}

// TLSVersionFromString converts a string representation of a TLS version.
func TLSVersionFromString(s string) (config.TLSVersion, error) {
	if s == "" {
		return 0, nil
	}
	if v, ok := config.TLSVersions[s]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unknown TLS version: %s", s)
}

// ToPrometheusConfig converts this object into the respective Prometheus configuration.
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
	ProxyConfig `json:",inline"`

	// ClientID is the public identifier for the client.
	// +optional
	ClientID string `json:"clientID"`
	// ClientSecret uses the secret as the client secret token.
	// +optional
	ClientSecret *SecretSelector `json:"clientSecret,omitempty"`
	// Scopes represents the scopes for the token request.
	// +optional
	Scopes []string `json:"scopes,omitempty"`
	// TokenURL is the URL to fetch the token from.
	// +optional
	TokenURL string `json:"tokenURL"`
	// EndpointParams are additional parameters to append to the token URL.
	// +optional
	EndpointParams map[string]string `json:"endpointParams,omitempty"`
	// TLS configures the token request's TLS settings.
	// +optional
	TLS *TLS `json:"tlsConfig,omitempty"`
}

// ToPrometheusConfig converts this object into the respective Prometheus configuration.
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
	if c.ProxyURL != "" {
		proxyConfig, err := c.ProxyConfig.ToPrometheusConfig()
		if err != nil {
			errs = append(errs, fmt.Errorf("OAuth2 proxy config: %w", err))
		} else {
			oauth2.ProxyURL = proxyConfig
		}
	}
	return oauth2, errors.Join(errs...)
}

// ProxyConfig specifies the proxy HTTP configuration.
type ProxyConfig struct {
	// ProxyURL is the HTTP proxy server to use to connect to the targets.
	//
	// Encoded passwords are not supported.
	// +optional
	// +kubebuilder:validation:MaxLength=2000
	// +kubebuilder:validation:XValidation:rule="isURL(self) && !self.matches('@')"
	ProxyURL string `json:"proxyUrl,omitempty"`

	// TODO(TheSpiritXIII): Consider adding further fields for Proxy configuration, similar to https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2
}

// ToPrometheusConfig converts this object into the respective Prometheus configuration.
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
// +kubebuilder:validation:XValidation:rule="((has(self.authorization) ? 1 : 0) + (has(self.basicAuth) ? 1 : 0) + (has(self.oauth2) ? 1 : 0)) <= 1"
type HTTPClientConfig struct {
	ProxyConfig `json:",inline"`

	// Authorization is the HTTP authorization credentials for the targets.
	// +optional
	Authorization *Auth `json:"authorization,omitempty"`
	// BasicAuth is the HTTP basic authentication credentials for the targets.
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
	// TLS configures the scrape request's TLS settings.
	// +optional
	TLS *TLS `json:"tls,omitempty"`
	// OAuth2 is the OAuth2 client credentials used to fetch a token for the targets.
	// +optional
	OAuth2 *OAuth2 `json:"oauth2,omitempty"`
}

// ToPrometheusConfig converts this object into the respective Prometheus configuration.
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
	if c.ProxyURL != "" {
		proxyConfig, err := c.ProxyConfig.ToPrometheusConfig()
		if err != nil {
			errs = append(errs, err)
		} else {
			clientConfig.ProxyURL = proxyConfig
		}
	}
	return clientConfig, errors.Join(errs...)
}
