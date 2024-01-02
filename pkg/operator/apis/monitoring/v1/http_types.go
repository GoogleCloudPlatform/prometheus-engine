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
)

// Auth sets the `Authorization` header on every scrape request.
//
// Currently the credentials are not configurable and always empty.
type Auth struct {
	// The authentication type. Defaults to Bearer, Basic will cause an error.
	Type string `json:"type,omitempty"`
	// TODO: Add credentials: https://github.com/GoogleCloudPlatform/prometheus-engine/issues/450
}

func (c *Auth) ToPrometheusConfig() *config.Authorization {
	return &config.Authorization{
		Type: c.Type,
	}
}

// BasicAuth sets the `Authorization` header on every scrape request with the
// configured username.
//
// Currently the password is not configurable and always empty.
type BasicAuth struct {
	// The username for authentication.
	Username string `json:"username,omitempty"`
	// TODO: Add password: https://github.com/GoogleCloudPlatform/prometheus-engine/issues/450
}

func (c *BasicAuth) ToPrometheusConfig() *config.BasicAuth {
	return &config.BasicAuth{
		Username: c.Username,
	}
}

// TLS specifies TLS configuration parameters from Kubernetes resources.
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

func (c *TLS) ToPrometheusConfig() (*config.TLSConfig, error) {
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
	return &config.TLSConfig{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
	}, nil
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
	// Proxy configuration.
	ProxyConfig `json:",inline"`
}

func (c *HTTPClientConfig) ToPrometheusConfig() (config.HTTPClientConfig, error) {
	var errs []error
	// Copy default config.
	clientConfig := config.DefaultHTTPClientConfig
	if c.Authorization != nil {
		clientConfig.Authorization = c.Authorization.ToPrometheusConfig()
	}
	if c.BasicAuth != nil {
		clientConfig.BasicAuth = c.BasicAuth.ToPrometheusConfig()
	}
	if c.TLS != nil {
		tlsConfig, err := c.TLS.ToPrometheusConfig()
		if err != nil {
			errs = append(errs, err)
		} else {
			clientConfig.TLSConfig = *tlsConfig
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
