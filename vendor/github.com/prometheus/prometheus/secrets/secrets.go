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
	"context"

	"github.com/go-kit/log"
)

// Secret represents a sensitive value.
type Secret interface {
	// Fetch fetches the secret content.
	Fetch(ctx context.Context) (string, error)
}

// SecretFn wraps a function to make it a Secret.
type SecretFn func(ctx context.Context) (string, error)

func (fn *SecretFn) Fetch(ctx context.Context) (string, error) {
	return (*fn)(ctx)
}

// Provider is a secret provider.
type Provider[T any] interface {
	// Add returns the secret fetcher for the given configuration.
	Add(ctx context.Context, config *T) (Secret, error)

	// Update returns the secret fetcher for the new configuration.
	Update(ctx context.Context, configBefore, configAfter *T) (Secret, error)

	// Remove ensures that the secret fetcher for the configuration is invalid.
	Remove(ctx context.Context, config *T) error
}

// ProviderFuncs is a secret provider.
type ProviderFuncs[T any] struct {
	// AddFunc returns the secret fetcher for the given configuration.
	AddFunc func(ctx context.Context, config T) (Secret, error)

	// UpdateFunc returns the secret fetcher for the new configuration.
	UpdateFunc func(ctx context.Context, configBefore, configAfter T) (Secret, error)

	// RemoveFunc ensures that the secret fetcher for the configuration is invalid.
	RemoveFunc func(ctx context.Context, config T) error
}

// Add implements Provider[T].
func (p *ProviderFuncs[T]) Add(ctx context.Context, config T) (Secret, error) {
	return p.AddFunc(ctx, config)
}

// Update implements Provider[T].
func (p *ProviderFuncs[T]) Update(ctx context.Context, configBefore, configAfter T) (Secret, error) {
	if p.UpdateFunc == nil {
		if err := p.Remove(ctx, configBefore); err != nil {
			return nil, err
		}
		return p.Add(ctx, configAfter)
	}
	return p.UpdateFunc(ctx, configBefore, configAfter)
}

// Remove implements Provider[T].
func (p *ProviderFuncs[T]) Remove(ctx context.Context, config T) error {
	if p.RemoveFunc == nil {
		return nil
	}
	return p.RemoveFunc(ctx, config)
}

// ProviderOptions provides options for a Provider.
type ProviderOptions struct {
	Logger log.Logger
}

// Config provides the configuration and constructor for a Provider.
type Config[T any] interface {
	// Name returns the name of the secret provider.
	Name() string

	// NewProvider creates a new Provider with the options.
	NewProvider(ctx context.Context, opts ProviderOptions) (Provider[T], error)
}

// // secretConfig is the YAML secret configuration.
// type secretConfig[T any] struct {
// 	Name   string `yaml:"name"`
// 	Config T      `yaml:"config"`
// }

// // providerConfig is the YAML secret provider configuration.
// type providerConfig[T any, U any] struct {
// 	Type    string
// 	Config  T                 `yaml:"config"`
// 	Secrets []secretConfig[U] `yaml:"secrets"`
// }

// // providerConfigEntry is the YAML secret provider entry configuration.
// type providerConfigEntry[T any, U any] struct {
// 	name   string
// 	config providerConfig[T, U]
// }

// func (p *providerConfigEntry[T, U]) entry() (string, providerConfig[T, U]) {
// 	return p.name, p.config
// }

// // Configs is a slice of Config values.
// type Configs []*providerConfig[Config[any], any]

// // UnmarshalYAML implements yaml.Unmarshaler.
// func (c *Configs) UnmarshalYAML(unmarshal func(interface{}) error) error {
// 	return c.unmarshalYAML(unmarshal, &globalRegistry)
// }

// func (c *Configs) unmarshalYAML(unmarshal func(interface{}) error, registry *Registry) error {
// 	var data []map[string]*providerConfig[yaml.Node, yaml.Node]
// 	if err := unmarshal(&data); err != nil {
// 		return err
// 	}

// 	var errs []error
// 	for _, configMap := range data {
// 		if len(configMap) != 1 {
// 			return fmt.Errorf("expected single mapping per item but found %d", len(configMap))
// 		}
// 		// For loop, but only 1 entry ever.
// 		for name, configAny := range configMap {
// 			config, err := registry.unmarshallYAML(name, configAny)
// 			if err != nil {
// 				errs = append(errs, err)
// 			}
// 			if config != nil {
// 				config.Type = name
// 				*c = append(*c, config)
// 			}
// 		}
// 	}

// 	return errors.Join(errs...)
// }

// // // MarshalYAML implements yaml.Marshaler.
// // func (c *Configs) MarshalYAML() (interface{}, error) {
// // 	var data []map[string]providerConfig[any, any]
// // 	for _, config := range *c {
// // 		m := make(map[string]providerConfig[any, any], 1)
// // 		m[config.name] = config.config
// // 		data = append(data, m)
// // 	}
// // 	return c, nil
// // }
