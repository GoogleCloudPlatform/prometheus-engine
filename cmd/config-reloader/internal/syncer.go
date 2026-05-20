// Copyright 2026 Google LLC
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

package internal

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ConfigMapSyncer materializes ConfigMaps matched by selector into files
// under outputDir. ConfigMaps whose name does not start with namePrefix are
// skipped.
type ConfigMapSyncer struct {
	client     kubernetes.Interface
	namespace  string
	selector   string
	namePrefix string
	outputDir  string
	logger     log.Logger
	interval   time.Duration

	lastHash string
}

// NewConfigMapSyncer constructs a syncer using in-cluster credentials.
// Empty namePrefix disables the name check.
func NewConfigMapSyncer(namespace, selector, namePrefix, outputDir string, interval time.Duration, logger log.Logger) (*ConfigMapSyncer, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return newConfigMapSyncerWithClient(client, namespace, selector, namePrefix, outputDir, interval, logger), nil
}

func newConfigMapSyncerWithClient(client kubernetes.Interface, namespace, selector, namePrefix, outputDir string, interval time.Duration, logger log.Logger) *ConfigMapSyncer {
	return &ConfigMapSyncer{
		client:     client,
		namespace:  namespace,
		selector:   selector,
		namePrefix: namePrefix,
		outputDir:  outputDir,
		interval:   interval,
		logger:     logger,
	}
}

// Sync runs one list-and-write cycle. It returns whether any file changed.
func (s *ConfigMapSyncer) Sync(ctx context.Context) (bool, error) {
	cmList, err := s.client.CoreV1().ConfigMaps(s.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: s.selector,
	})
	if err != nil {
		return false, fmt.Errorf("list configmaps: %w", err)
	}

	files := make(map[string][]byte)
	for i := range cmList.Items {
		cm := &cmList.Items[i]
		if s.namePrefix != "" && !strings.HasPrefix(cm.Name, s.namePrefix) {
			//nolint:errcheck
			level.Warn(s.logger).Log("msg", "ignoring configmap with unexpected name", "name", cm.Name, "want_prefix", s.namePrefix)
			continue
		}
		for k, v := range cm.Data {
			files[cm.Name+"__"+k] = []byte(v)
		}
		for k, v := range cm.BinaryData {
			files[cm.Name+"__"+k] = v
		}
	}

	hash := hashFiles(files)
	if hash == s.lastHash {
		return false, nil
	}

	if err := s.writeFiles(files); err != nil {
		return false, err
	}

	s.lastHash = hash
	//nolint:errcheck
	level.Info(s.logger).Log("msg", "synced configmap rules", "configmaps", len(cmList.Items), "files", len(files))
	return true, nil
}

// Run does an initial Sync and then re-syncs on every interval until ctx is cancelled.
func (s *ConfigMapSyncer) Run(ctx context.Context) error {
	// Best-effort initial sync; the reloader will pick up files on its next poll cycle.
	if _, err := s.Sync(ctx); err != nil {
		//nolint:errcheck
		level.Warn(s.logger).Log("msg", "initial configmap sync failed", "err", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := s.Sync(ctx); err != nil {
				//nolint:errcheck
				level.Warn(s.logger).Log("msg", "configmap sync failed", "err", err)
			}
		}
	}
}

func (s *ConfigMapSyncer) writeFiles(files map[string][]byte) error {
	if err := os.MkdirAll(s.outputDir, 0o755); err != nil {
		return err
	}

	for name, data := range files {
		if filepath.Base(name) != name {
			continue
		}
		if err := os.WriteFile(filepath.Join(s.outputDir, name), data, 0o644); err != nil {
			return err
		}
	}

	entries, err := os.ReadDir(s.outputDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := files[e.Name()]; !ok {
			if err := os.Remove(filepath.Join(s.outputDir, e.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func hashFiles(files map[string][]byte) string {
	h := sha256.New()

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00", k)
		_, _ = h.Write(files[k])
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
