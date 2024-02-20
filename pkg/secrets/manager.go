// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"
)

var (
	failedSecretConfigs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "prometheus_kubernetes_failed_secret_configs",
			Help: "Current number of secret configurations that failed to load.",
		},
	)
	secretsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "prometheus_kubernetes_secrets_total",
			Help: "Current number of secrets.",
		},
	)
)

func yamlSerialize(obj any) ([]byte, error) {
	if obj == nil {
		return []byte{}, nil
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	if err := encoder.Encode(obj); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func yamlEqual(x, y any) (bool, error) {
	yamlX, err := yamlSerialize(x)
	if err != nil {
		return false, err
	}
	yamlY, err := yamlSerialize(y)
	if err != nil {
		return false, err
	}
	return bytes.Equal(yamlX, yamlY), nil
}

// SecretConfig maps a secret name references to a Kubernetes secret.
type SecretConfig struct {
	Name   string                 `yaml:"name"`
	Config KubernetesSecretConfig `yaml:"config"`
}

type secretEntry struct {
	config KubernetesSecretConfig
	secret Secret
}

// Manager manages the Kubernetes secret provider.
type Manager struct {
	ctx  context.Context
	opts ProviderOptions
	mtx  sync.Mutex

	cancelFn func()
	provider *watchProvider
	config   *WatchSPConfig
	secrets  map[string]*secretEntry
}

// NewManager creates a new secret manager with the provided options.
func NewManager(ctx context.Context, reg prometheus.Registerer, opts ProviderOptions) Manager {
	if reg != nil {
		reg.MustRegister(failedSecretConfigs)
		reg.MustRegister(secretsTotal)
	}
	// Note, we do not create the Kubernetes client until we have secrets.
	return Manager{
		ctx:      ctx,
		cancelFn: func() {},
		secrets:  make(map[string]*secretEntry),
		opts:     opts,
	}
}

// ApplyConfig applies the new secrets, diffing each one with the last configuration to apply the
// relevant update.
func (m *Manager) ApplyConfig(providerConfig *WatchSPConfig, configs []SecretConfig) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// If no secrets are provided, cancel any existing secret provider.
	if len(configs) == 0 {
		m.cancelFn()
		m.provider = nil
		m.cancelFn = func() {}
		m.secrets = map[string]*secretEntry{}
		m.config = nil
		return nil
	}

	// We must recreate the Kubernetes client and reconnect all secrets if the client configuration
	// changes. Hypothetically this could be a different API server (or just different parameters).
	eq, err := yamlEqual(m.config, providerConfig)
	if err != nil {
		return err
	}

	defer func() {
		m.config = providerConfig
	}()

	// We may have an empty Kubernetes configuration (indicating default parameters). Since we don't
	// have a client until we have secrets, we must create one now, or recreate it if the
	// configuration changed.
	if !eq || m.provider == nil {
		ctx, cancel := context.WithCancel(m.ctx)
		provider, err := providerConfig.newProvider(ctx, m.opts)
		if err != nil {
			cancel()
			return err
		}

		m.cancelFn()
		m.provider = provider
		m.cancelFn = cancel
		m.secrets = map[string]*secretEntry{}
	}
	return m.updateSecrets(configs)
}

func (m *Manager) updateSecrets(configs []SecretConfig) error {
	var errs []error

	// Do a first pass to check for errors and disable those secrets.
	secretNamesEnabled := make(map[string]bool)
	for _, secret := range configs {
		if enabled, ok := secretNamesEnabled[secret.Name]; ok {
			if !enabled {
				continue
			}
			errs = append(errs, fmt.Errorf("duplicate secret key %q", secret.Name))
			secretNamesEnabled[secret.Name] = false
		} else {
			secretNamesEnabled[secret.Name] = true
		}
	}

	secretsFinal := map[string]*secretEntry{}
	for i := range configs {
		secretIncoming := &configs[i]
		if enabled := secretNamesEnabled[secretIncoming.Name]; !enabled {
			continue
		}
		// First check if we've registered this secret before.
		if secretPrevious, ok := m.secrets[secretIncoming.Name]; ok {
			// Track all the secrets we saw. The leftover are later removed.
			delete(m.secrets, secretIncoming.Name)

			// If the config didn't change, we skip this one.
			eq, err := yamlEqual(&secretPrevious.config, &secretIncoming.Config)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if eq {
				secretsFinal[secretIncoming.Name] = secretPrevious
				continue
			}

			// The config changed, so update it.
			s, err := m.provider.Update(&secretPrevious.config, &secretIncoming.Config)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			secretPrevious.secret = s
			secretsFinal[secretIncoming.Name] = secretPrevious
		} else {
			// We've never seen this secret before, so add it.
			s, err := m.provider.Add(&secretIncoming.Config)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			secretsFinal[secretIncoming.Name] = &secretEntry{
				config: secretIncoming.Config,
				secret: s,
			}
		}
	}
	for _, secretUnused := range m.secrets {
		m.provider.Remove(&secretUnused.config)
	}

	m.secrets = secretsFinal

	total := len(secretNamesEnabled)
	success := len(m.secrets)
	failedSecretConfigs.Set(float64(total - success))
	secretsTotal.Set(float64(total))
	return errors.Join(errs...)
}

// Fetch implements github.com/prometheus/common/config.SecretManager.Fetch.
func (m *Manager) Fetch(ctx context.Context, name string) (string, error) {
	secret, ok := m.secrets[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}
	return secret.secret.Fetch(ctx)
}

// Close cancels the manager, stopping the Kubernetes secret provider.
func (m *Manager) Close(reg prometheus.Registerer) {
	m.cancelFn()
	if reg != nil {
		reg.Unregister(failedSecretConfigs)
		reg.Unregister(secretsTotal)
	}
}
