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
	"github.com/prometheus/prometheus/secrets"
	"github.com/prometheus/prometheus/secrets/kubernetes"
)

type SecretSelector struct {
	// The local secret and key to use.
	Secret *SecretKeySelector `json:"secret,omitempty"`
}

func (s *SecretSelector) GetSecretKey(pm PodMonitoringCRD) string {
	if s.Secret == nil {
		return ""
	}
	return s.Secret.GetLocalSecretKey(pm.GetNamespace())
}

func (s *SecretSelector) GetSecret(pm PodMonitoringCRD) *secrets.SecretConfig[kubernetes.SecretConfig] {
	if s.Secret == nil {
		return nil
	}
	secret := s.Secret.GetLocalSecret(pm.GetNamespace())
	return &secret
}

type SecretKeySelector struct {
	// The name of the secret to select from.
	Name string `json:"name"`

	// The key of the secret to select from. Must be a valid secret key.
	Key string `json:"key"`
}

func (s *SecretKeySelector) GetLocalSecretKey(namespace string) string {
	return fmt.Sprintf("%s/%s/%s", namespace, s.Name, s.Key)
}

func (s *SecretKeySelector) GetLocalSecret(namespace string) secrets.SecretConfig[kubernetes.SecretConfig] {
	return secrets.SecretConfig[kubernetes.SecretConfig]{
		Name: s.GetLocalSecretKey(namespace),
		Config: kubernetes.SecretConfig{
			Namespace: namespace,
			Name:      s.Name,
			Key:       s.Key,
		},
	}
}

type ClusterSecretSelector struct {
	// The secret and key to use.
	Secret *ClusterSecretKeySelector `json:"secret,omitempty"`
}

func (s *ClusterSecretSelector) GetSecretKey(pm PodMonitoringCRD) string {
	return s.GetClusterSecretKey()
}

func (s *ClusterSecretSelector) GetSecret(pm PodMonitoringCRD) *secrets.SecretConfig[kubernetes.SecretConfig] {
	return s.GetClusterSecret()
}

func (s *ClusterSecretSelector) GetClusterSecretKey() string {
	if s.Secret == nil {
		return ""
	}
	return s.Secret.GetClusterSecretKey()
}

func (s *ClusterSecretSelector) GetClusterSecret() *secrets.SecretConfig[kubernetes.SecretConfig] {
	if s.Secret == nil {
		return nil
	}
	secret := s.Secret.GetClusterSecret()
	return &secret
}

type ClusterSecretKeySelector struct {
	// The secret name and key to retrieve (inlined).
	SecretKeySelector `json:",inline"`

	// The namespace to retrieve the secret from.
	Namespace string `json:"namespace"`
}

func (s *ClusterSecretKeySelector) GetClusterSecretKey() string {
	return s.SecretKeySelector.GetLocalSecretKey(s.Namespace)
}

func (s *ClusterSecretKeySelector) GetClusterSecret() secrets.SecretConfig[kubernetes.SecretConfig] {
	return s.SecretKeySelector.GetLocalSecret(s.Namespace)
}

// Auth sets the `Authorization` header on every scrape request.
//
// Currently the credentials are not configurable and always empty.
type Auth struct {
	// The authentication type. Defaults to Bearer, Basic will cause an error.
	Type        string                `json:"type,omitempty"`
	Credentials ClusterSecretSelector `json:"credentials,omitempty"`
}

func (c *Auth) ToPrometheusConfig(pm PodMonitoringCRD) *config.Authorization {
	auth := &config.Authorization{
		Type:           c.Type,
		CredentialsRef: c.Credentials.GetSecretKey(pm),
	}
	return auth
}

func (c *Auth) GetSecretConfig() *secrets.SecretConfig[kubernetes.SecretConfig] {
	return c.Credentials.GetClusterSecret()
}

// BasicAuth sets the `Authorization` header on every scrape request with the
// configured username.
//
// Currently the password is not configurable and always empty.
type BasicAuth struct {
	// The username for authentication.
	Username string `json:"username,omitempty"`
	// The password for authentication.
	Password ClusterSecretSelector `json:"password"`
}

func (c *BasicAuth) ToPrometheusConfig(pm PodMonitoringCRD) *config.BasicAuth {
	basicAuth := &config.BasicAuth{
		Username:    c.Username,
		PasswordRef: c.Password.GetSecretKey(pm),
	}
	return basicAuth
}

func (c *BasicAuth) GetSecretConfig() *secrets.SecretConfig[kubernetes.SecretConfig] {
	return c.Password.GetClusterSecret()
}

// TLS specifies TLS configuration parameters from Kubernetes resources.
type TLS struct {
	// TODO
	CA ClusterSecretSelector `json:"ca"`
	// TODO
	Cert ClusterSecretSelector `json:"cert"`
	// TODO
	KeySecret ClusterSecretSelector `json:"keySecret"`
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

func (c *TLS) ToPrometheusConfig(pm PodMonitoringCRD) (*config.TLSConfig, error) {
	var errs []error
	minVersion, err := TLSVersionFromString(c.MinVersion)
	if err != nil {
		errs = append(errs, fmt.Errorf("unable to convert TLS min version: %w", err))
	}
	maxVersion, err := TLSVersionFromString(c.MaxVersion)
	if err != nil {
		errs = append(errs, fmt.Errorf("unable to convert TLS min version: %w", err))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	tls := &config.TLSConfig{
		CARef:              c.CA.GetSecretKey(pm),
		CertRef:            c.Cert.GetSecretKey(pm),
		KeyRef:             c.KeySecret.GetSecretKey(pm),
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
	}
	return tls, nil
}

func (c *TLS) GetSecretConfig() []secrets.SecretConfig[kubernetes.SecretConfig] {
	var secrets []secrets.SecretConfig[kubernetes.SecretConfig]
	if secret := c.CA.GetClusterSecret(); secret != nil {
		secrets = append(secrets, *secret)
	}
	if secret := c.Cert.GetClusterSecret(); secret != nil {
		secrets = append(secrets, *secret)
	}
	if secret := c.KeySecret.GetClusterSecret(); secret != nil {
		secrets = append(secrets, *secret)
	}
	return secrets
}

// OAuth2 is the OAuth2 client configuration.
//
// Currently the client secret is not configurable and always empty.
type OAuth2 struct {
	// Public identifier for the client.
	ClientID string `json:"clientID"`
	// TODO
	ClientSecret ClusterSecretSelector `json:"clientSecret"`
	// Scopes for the token request.
	Scopes []string `json:"scopes,omitempty"`
	// The URL to fetch the token from.
	TokenURL string `json:"tokenURL"`
	// Optional parameters to append to the token URL.
	EndpointParams map[string]string `json:"endpointParams,omitempty"`
	// Configures the token request's TLS settings.
	TLS         *TLS `json:"tlsConfig,omitempty"`
	ProxyConfig `json:",inline"`
	// TODO: Add ClientSecret: https://github.com/GoogleCloudPlatform/prometheus-engine/issues/450
}

func (c *OAuth2) ToPrometheusConfig(pm PodMonitoringCRD) (*config.OAuth2, error) {
	oauth2 := &config.OAuth2{
		ClientID:        c.ClientID,
		ClientSecretRef: c.ClientSecret.GetSecretKey(pm),
		Scopes:          c.Scopes,
		TokenURL:        c.TokenURL,
		EndpointParams:  c.EndpointParams,
	}
	if c.TLS != nil {
		tlsConfig, err := c.TLS.ToPrometheusConfig(pm)
		if err != nil {
			return nil, fmt.Errorf("OAuth2 TLS: %w", err)
		}
		oauth2.TLSConfig = *tlsConfig
	}
	if c.ProxyConfig.ProxyURL != "" {
		proxyConfig, err := c.ProxyConfig.ToPrometheusConfig()
		if err != nil {
			return nil, fmt.Errorf("OAuth2 proxy config: %w", err)
		}
		oauth2.ProxyURL = proxyConfig
	}
	return oauth2, nil
}

func (c *OAuth2) GetSecretConfig() []secrets.SecretConfig[kubernetes.SecretConfig] {
	var secrets []secrets.SecretConfig[kubernetes.SecretConfig]
	if secret := c.ClientSecret.GetClusterSecret(); secret != nil {
		secrets = append(secrets, *secret)
	}
	if c.TLS != nil {
		secrets = append(secrets, c.TLS.GetSecretConfig()...)
	}
	return secrets
}

type ProxyConfig struct {
	// HTTP proxy server to use to connect to the targets. Encoded passwords are not supported.
	ProxyURL string `json:"proxyUrl,omitempty"`
	// TODO(TheSpiritXIII): https://prometheus.io/docs/prometheus/latest/configuration/configuration/#oauth2
}

func (c *ProxyConfig) ToPrometheusConfig() (config.URL, error) {
	proxyURL, err := url.Parse(c.ProxyURL)
	if err != nil {
		return config.URL{}, fmt.Errorf("invalid proxy URL: %w", err)
	}
	// Marshalling the config will redact the password, so we don't support those.
	// It's not a good idea anyway and we will later support basic auth based on secrets to
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

func (c *HTTPClientConfig) ToPrometheusConfig(pm PodMonitoringCRD) (config.HTTPClientConfig, error) {
	var errs []error
	// Copy default config.
	clientConfig := config.DefaultHTTPClientConfig
	if c.Authorization != nil {
		clientConfig.Authorization = c.Authorization.ToPrometheusConfig(pm)
	}
	if c.BasicAuth != nil {
		clientConfig.BasicAuth = c.BasicAuth.ToPrometheusConfig(pm)
	}
	if c.TLS != nil {
		tlsConfig, err := c.TLS.ToPrometheusConfig(pm)
		if err != nil {
			errs = append(errs, err)
		} else {
			clientConfig.TLSConfig = *tlsConfig
		}
	}
	if c.OAuth2 != nil {
		oauth2, err := c.OAuth2.ToPrometheusConfig(pm)
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

func (c *HTTPClientConfig) GetSecretConfigs() []secrets.SecretConfig[kubernetes.SecretConfig] {
	var secrets []secrets.SecretConfig[kubernetes.SecretConfig]
	if c.Authorization != nil {
		if secret := c.Authorization.GetSecretConfig(); secret != nil {
			secrets = append(secrets, *secret)
		}
	}
	if c.BasicAuth != nil {
		if secret := c.BasicAuth.GetSecretConfig(); secret != nil {
			secrets = append(secrets, *secret)
		}
	}
	if c.TLS != nil {
		secrets = append(secrets, c.TLS.GetSecretConfig()...)
	}
	if c.OAuth2 != nil {
		secrets = append(secrets, c.OAuth2.GetSecretConfig()...)
	}
	return secrets
}
