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
	"fmt"

	"github.com/prometheus/prometheus/secrets"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type OnDemandSPConfig struct {
	ClientConfig
}

// Name returns the name of the Config.
func (*OnDemandSPConfig) Name() string { return "kubernetes_on_demand" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *OnDemandSPConfig) NewProvider(ctx context.Context, opts secrets.ProviderOptions) (secrets.Provider[SecretConfig], error) {
	client, err := c.ClientConfig.client()
	if err != nil {
		return nil, err
	}
	return newOnDemandProvider(client)
}

func newOnDemandProvider(client kubernetes.Interface) (secrets.Provider[SecretConfig], error) {
	return &secrets.ProviderFuncs[*SecretConfig]{
		AddFunc: func(ctx context.Context, config *SecretConfig) (secrets.Secret, error) {
			provider := secrets.SecretFn(func(ctx context.Context) (string, error) {
				secret, err := client.CoreV1().Secrets(config.Namespace).Get(ctx, config.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return "", fmt.Errorf("secret %s/%s not found", config.Namespace, config.Name)
					}
					return "", fmt.Errorf("secret %s/%s API: %w", config.Namespace, config.Name, err)
				}
				return getKey(secret, config.Key)
			})
			return &provider, nil
		},
	}, nil
}
