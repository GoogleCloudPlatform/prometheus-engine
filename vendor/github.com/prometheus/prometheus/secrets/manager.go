// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
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

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"
)

var (
	failedSecretConfigs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "prometheus_sp_failed_secret_configs",
			Help: "Current number of secret configurations that failed to load.",
		},
	)
	secretsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "prometheus_sp_secrets_total",
			Help: "Current number of secrets.",
		},
	)
)

func init() {
	prometheus.MustRegister(failedSecretConfigs)
	prometheus.MustRegister(secretsTotal)
}

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

type SecretConfig[T any] struct {
	Name   string `yaml:"name"`
	Config T      `yaml:"config"`
}

type SecretEntry[T any] struct {
	config T
	secret Secret
}

type ProviderManager[T any] struct {
	ctx      context.Context
	cancelFn func()
	provider Provider[T]
	config   Config[T]
	secrets  map[string]*SecretEntry[T]
}

func NewProviderManager[T any](ctx context.Context) ProviderManager[T] {
	m := ProviderManager[T]{
		ctx:      ctx,
		cancelFn: func() {},
		secrets:  make(map[string]*SecretEntry[T]),
	}
	return m
}

func (m *ProviderManager[T]) ApplyConfig(ctx context.Context, providerConfig Config[T], configs []SecretConfig[T]) error {
	eq, err := yamlEqual(m.config, providerConfig)
	if err != nil {
		return err
	}

	defer func() {
		m.config = providerConfig
	}()

	if !eq {
		ctx, cancel := context.WithCancel(m.ctx)
		provider, err := providerConfig.NewProvider(ctx, ProviderOptions{})
		if err != nil {
			cancel()
			return err
		}

		m.cancelFn()
		m.provider = provider
		m.cancelFn = cancel
		m.secrets = map[string]*SecretEntry[T]{}
	}
	return m.updateSecrets(ctx, configs)
}

func (m *ProviderManager[T]) updateSecrets(ctx context.Context, configs []SecretConfig[T]) error {
	var errs []error
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

	newSecrets := map[string]*SecretEntry[T]{}
	for _, secret := range configs {
		if enabled := secretNamesEnabled[secret.Name]; !enabled {
			continue
		}
		if secretConfig, ok := m.secrets[secret.Name]; ok {
			eq, err := yamlEqual(secretConfig.config, secret.Config)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			delete(m.secrets, secret.Name)
			if eq {
				newSecrets[secret.Name] = m.secrets[secret.Name]
				continue
			}
			s, err := m.provider.Update(ctx, &secretConfig.config, &secret.Config)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			secretConfig.secret = s
			newSecrets[secret.Name] = secretConfig
			continue
		}
		s, err := m.provider.Add(ctx, &secret.Config)
		if err != nil {
			errs = append(errs, err)
			break
		}
		newSecrets[secret.Name] = &SecretEntry[T]{
			config: secret.Config,
			secret: s,
		}
	}
	for _, unusedSecret := range m.secrets {
		if err := m.provider.Remove(ctx, &unusedSecret.config); err != nil {
			errs = append(errs, err)
		}
	}

	m.secrets = newSecrets

	total := len(secretNamesEnabled)
	success := len(m.secrets)
	failedSecretConfigs.Set(float64(total - success))
	secretsTotal.Set(float64(total))
	return errors.Join(errs...)
}

func (m *ProviderManager[T]) GetSecret(ctx context.Context, name string) (string, error) {
	secret, ok := m.secrets[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}
	return secret.secret.Fetch(ctx)
}
