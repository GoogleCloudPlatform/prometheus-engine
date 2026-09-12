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
	"cmp"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	dataDirLink  = "..data"
	dataTmpLink  = "..data_tmp"
	hiddenPrefix = ".."
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

	shards := make([]*corev1.ConfigMap, 0, len(cmList.Items))
	for i := range cmList.Items {
		shards = append(shards, &cmList.Items[i])
	}
	slices.SortFunc(shards, func(a, b *corev1.ConfigMap) int {
		return cmp.Compare(a.Name, b.Name)
	})

	// Keys are unique across shards, so leaving the shard out of the file name means re-sharding never renames a file.
	files := make(map[string][]byte)
	origin := make(map[string]string)
	for _, cm := range shards {
		if s.namePrefix != "" && !strings.HasPrefix(cm.Name, s.namePrefix) {
			level.Warn(s.logger).Log("msg", "ignoring configmap with unexpected name", "name", cm.Name, "want_prefix", s.namePrefix)
			continue
		}
		for k, v := range cm.Data {
			s.addFile(files, origin, cm.Name, k, []byte(v))
		}
		for k, v := range cm.BinaryData {
			s.addFile(files, origin, cm.Name, k, v)
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
	level.Info(s.logger).Log("msg", "synced configmap rules", "configmaps", len(cmList.Items), "files", len(files))
	return true, nil
}

// Run does an initial Sync and then re-syncs on every interval until ctx is cancelled.
func (s *ConfigMapSyncer) Run(ctx context.Context) error {
	// Best-effort initial sync; the reloader will pick up files on its next poll cycle.
	if _, err := s.Sync(ctx); err != nil {
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
				level.Warn(s.logger).Log("msg", "configmap sync failed", "err", err)
			}
		}
	}
}

func (s *ConfigMapSyncer) addFile(files map[string][]byte, origin map[string]string, cmName, key string, data []byte) {
	if filepath.Base(key) != key || strings.HasPrefix(key, ".") {
		level.Warn(s.logger).Log("msg", "skipping invalid filename", "configmap", cmName, "key", key)
		return
	}
	if prev, ok := origin[key]; ok {
		level.Warn(s.logger).Log("msg", "duplicate key across shards, keeping first", "key", key, "kept", prev, "skipped", cmName)
		return
	}
	files[key] = data
	origin[key] = cmName
}

// Staging in a fresh payload dir and flipping ..data with one rename keeps a reader listing outputDir from ever seeing a half-written set.
func (s *ConfigMapSyncer) writeFiles(files map[string][]byte) error {
	if err := os.MkdirAll(s.outputDir, 0o755); err != nil {
		return err
	}

	payload := hiddenPrefix + time.Now().UTC().Format("2006_01_02_15_04_05.000000000")
	payloadDir := filepath.Join(s.outputDir, payload)
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		return err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(payloadDir, name), data, 0o644); err != nil {
			return err
		}
	}

	// Drop links before the swap, so no link is ever left dangling.
	if err := s.removeStaleLinks(files); err != nil {
		return err
	}
	if err := s.swapDataLink(payload); err != nil {
		return err
	}
	if err := s.linkFiles(files); err != nil {
		return err
	}
	return s.removeOldPayloads(payload)
}

func (s *ConfigMapSyncer) removeStaleLinks(files map[string][]byte) error {
	entries, err := os.ReadDir(s.outputDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, hiddenPrefix) {
			continue
		}
		if _, ok := files[name]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.outputDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func (s *ConfigMapSyncer) swapDataLink(payload string) error {
	tmp := filepath.Join(s.outputDir, dataTmpLink)
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(payload, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.outputDir, dataDirLink))
}

func (s *ConfigMapSyncer) linkFiles(files map[string][]byte) error {
	for name := range files {
		path := filepath.Join(s.outputDir, name)
		target := filepath.Join(dataDirLink, name)

		if cur, err := os.Readlink(path); err == nil && cur == target {
			continue
		}
		tmp := path + ".tmp"
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Symlink(target, tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	return nil
}

func (s *ConfigMapSyncer) removeOldPayloads(current string) error {
	entries, err := os.ReadDir(s.outputDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || !strings.HasPrefix(name, hiddenPrefix) || name == current {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.outputDir, name)); err != nil {
			return err
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
	slices.Sort(keys)

	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00", k)
		_, _ = h.Write(files[k])
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
