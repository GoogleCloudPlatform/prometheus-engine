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

package kubernetes

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/secrets"
)

type SecretConfigs []secrets.SecretConfig[SecretConfig]

type Manager struct {
	ctx      context.Context
	provider secrets.ProviderManager[SecretConfig]
}

func NewManager(ctx context.Context, log log.Logger) *Manager {
	return &Manager{
		ctx:      ctx,
		provider: secrets.NewProviderManager[SecretConfig](ctx),
	}
}

// ApplyConfig checks if secret provider with supplied config is already running and keeps them as is.
// Remaining providers are then stopped and new required providers are started using the provided config.
func (m *Manager) ApplyConfig(ctx context.Context, providerConfig secrets.Config[SecretConfig], configs SecretConfigs) error {
	return m.provider.ApplyConfig(ctx, providerConfig, configs)
}

func (m *Manager) Fetch(ctx context.Context, name string) (string, error) {
	return m.provider.GetSecret(m.ctx, name)
}
