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
	"sync"

	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/secrets"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type WatchSPConfig struct {
	ClientConfig
}

// Name returns the name of the Config.
func (*WatchSPConfig) Name() string { return "kubernetes_watch" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *WatchSPConfig) NewProvider(ctx context.Context, opts secrets.ProviderOptions) (secrets.Provider[SecretConfig], error) {
	client, err := c.ClientConfig.client()
	if err != nil {
		return nil, err
	}
	return newWatchProvider(ctx, opts.Logger, client)
}

type watcher struct {
	mu       sync.Mutex
	w        watch.Interface
	refCount uint
	s        *corev1.Secret
}

func newWatcher(ctx context.Context, logger log.Logger, client kubernetes.Interface, config *SecretConfig) (*watcher, error) {
	val := &watcher{
		refCount: 1,
	}
	var err error
	val.w, err = client.CoreV1().Secrets(config.Namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("name=%s", config.Name),
	})
	if err != nil {
		return val, err
	}
	val.s, err = client.CoreV1().Secrets(config.Namespace).Get(ctx, config.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return val, err
	}
	go func() {
		for {
			select {
			case e := <-val.w.ResultChan():
				val.update(ctx, logger, client, e)
			case <-ctx.Done():
				val.w.Stop()
				return
			}
		}
	}()

	return val, nil
}

func (w *watcher) update(ctx context.Context, logger log.Logger, client kubernetes.Interface, e watch.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if e.Type == "" && e.Object == nil {
		return
	}
	switch e.Type {
	case watch.Modified, watch.Added:
		secret := e.Object.(*corev1.Secret)
		w.s = secret
	case watch.Deleted:
		w.s = nil
	case watch.Bookmark:
		secret := e.Object.(*corev1.Secret)
		if w.s.ResourceVersion != secret.ResourceVersion {
			var err error
			w.s, err = client.CoreV1().Secrets(w.s.Namespace).Get(ctx, w.s.Name, metav1.GetOptions{
				ResourceVersion: secret.ResourceVersion,
			})
			if err != nil {
				logger.Log("msg", "failed to get secret", "err", err, "namespace", w.s.Namespace, "name", w.s.Name)
				return
			}
		}
	case watch.Error:
		logger.Log("msg", "watch error event", "namespace", w.s.Namespace, "name", w.s.Name)
	}
}

func (w *watcher) secret(config *SecretConfig) secrets.Secret {
	fn := secrets.SecretFn(func(ctx context.Context) (string, error) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.s == nil {
			return "", fmt.Errorf("secret %s/%s not found", config.Namespace, config.Name)
		}
		return getKey(w.s, config.Key)
	})
	return &fn
}

type watchProvider struct {
	ctx                context.Context
	mu                 sync.Mutex
	client             kubernetes.Interface
	secretKeyToWatcher map[string]*watcher
	logger             log.Logger
}

func newWatchProvider(ctx context.Context, logger log.Logger, client kubernetes.Interface) (*watchProvider, error) {
	return &watchProvider{
		ctx:                ctx,
		mu:                 sync.Mutex{},
		client:             client,
		secretKeyToWatcher: map[string]*watcher{},
		logger:             logger,
	}, nil
}

func (p *watchProvider) Add(ctx context.Context, config *SecretConfig) (secrets.Secret, error) {
	keyStr := config.objectKey().String()
	p.mu.Lock()
	defer p.mu.Unlock()

	val, ok := p.secretKeyToWatcher[keyStr]
	if ok {
		val.refCount += 1
		return nil, nil
	}

	var err error
	val, err = newWatcher(ctx, p.logger, p.client, config)
	if err != nil {
		return nil, err
	}

	p.secretKeyToWatcher[keyStr] = val
	return val.secret(config), nil
}

func (p *watchProvider) Update(ctx context.Context, configBefore, configAfter *SecretConfig) (secrets.Secret, error) {
	keyBefore := configBefore.objectKey()
	keyAfter := configAfter.objectKey()
	if keyBefore == keyAfter {
		val := p.secretKeyToWatcher[keyAfter.String()]
		if val == nil {
			return nil, fmt.Errorf("secret %s/%s not found", configAfter.Namespace, configAfter.Name)
		}
		return val.secret(configAfter), nil
	}
	if err := p.Remove(ctx, configBefore); err != nil {
		return nil, err
	}
	return p.Add(ctx, configAfter)
}

func (p *watchProvider) Remove(ctx context.Context, config *SecretConfig) error {
	key := config.objectKey().String()
	val := p.secretKeyToWatcher[key]
	if val == nil {
		return nil
	}
	val.refCount -= 1
	if val.refCount > 0 {
		return nil
	}
	delete(p.secretKeyToWatcher, key)
	val.w.Stop()
	return nil
}

func (p *watchProvider) isClean() bool {
	return len(p.secretKeyToWatcher) == 0
}
