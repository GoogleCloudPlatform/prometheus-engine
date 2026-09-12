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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const shardSelector = "monitoring.googleapis.com/rules-shard=true"

func newTestSyncer(t *testing.T, outputDir string, objs ...*corev1.ConfigMap) (*ConfigMapSyncer, *fake.Clientset) {
	t.Helper()
	client := fake.NewSimpleClientset()
	for _, o := range objs {
		if _, err := client.CoreV1().ConfigMaps(o.Namespace).Create(t.Context(), o, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return newConfigMapSyncerWithClient(client, "gmp-system", shardSelector, "rules-generated-", outputDir, time.Second, log.NewNopLogger()), client
}

func shardConfigMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "gmp-system",
			Labels:    map[string]string{"monitoring.googleapis.com/rules-shard": "true"},
		},
		Data: data,
	}
}

func visibleFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "..") {
			continue
		}
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

func readVisible(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func payloadDirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "..") {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestConfigMapSyncer_BasicSync(t *testing.T) {
	outputDir := t.TempDir()
	syncer, _ := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"rules__default__test.yaml": "groups:\n- name: test\n  rules: []\n",
	}))

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on first sync")
	}

	if got := readVisible(t, outputDir, "rules__default__test.yaml"); got != "groups:\n- name: test\n  rules: []\n" {
		t.Errorf("unexpected file content: %q", got)
	}
}

func TestConfigMapSyncer_AtomicLayout(t *testing.T) {
	outputDir := t.TempDir()
	syncer, _ := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"rules__default__a.yaml": "a",
		"rules__default__b.yaml": "b",
	}))

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	target, err := os.Readlink(filepath.Join(outputDir, "..data"))
	if err != nil {
		t.Fatalf("..data is not a symlink: %v", err)
	}
	if !strings.HasPrefix(target, "..") {
		t.Errorf("..data points at %q, want a payload dir", target)
	}

	for _, name := range visibleFiles(t, outputDir) {
		link, err := os.Readlink(filepath.Join(outputDir, name))
		if err != nil {
			t.Errorf("%s is not a symlink: %v", name, err)
			continue
		}
		if want := filepath.Join("..data", name); link != want {
			t.Errorf("%s points at %q, want %q", name, link, want)
		}
	}

	if dirs := payloadDirs(t, outputDir); len(dirs) != 1 {
		t.Errorf("expected 1 payload dir, got %v", dirs)
	}
}

// A renamed file would make the rule evaluator briefly see the group twice.
func TestConfigMapSyncer_ReshardKeepsFileNames(t *testing.T) {
	outputDir := t.TempDir()
	syncer, client := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"rules__default__a.yaml": "a",
		"rules__default__b.yaml": "b",
	}))

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	before := visibleFiles(t, outputDir)

	if _, err := client.CoreV1().ConfigMaps("gmp-system").Update(t.Context(),
		shardConfigMap("rules-generated-0", map[string]string{"rules__default__a.yaml": "a"}),
		metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ConfigMaps("gmp-system").Create(t.Context(),
		shardConfigMap("rules-generated-1", map[string]string{"rules__default__b.yaml": "b"}),
		metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("expected changed=false: re-sharding moved no content")
	}

	after := visibleFiles(t, outputDir)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("file set changed across re-shard: %v -> %v", before, after)
	}
}

func TestConfigMapSyncer_DuplicateKeyAcrossShards(t *testing.T) {
	outputDir := t.TempDir()
	syncer, _ := newTestSyncer(t, outputDir,
		shardConfigMap("rules-generated-0", map[string]string{"rules__default__a.yaml": "first"}),
		shardConfigMap("rules-generated-1", map[string]string{"rules__default__a.yaml": "second"}),
	)

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := visibleFiles(t, outputDir); len(got) != 1 {
		t.Fatalf("expected 1 file, got %v", got)
	}
	if got := readVisible(t, outputDir, "rules__default__a.yaml"); got != "first" {
		t.Errorf("expected lowest shard to win, got %q", got)
	}
}

func TestConfigMapSyncer_NoChangeOnSecondSync(t *testing.T) {
	outputDir := t.TempDir()
	syncer, _ := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"test.yaml": "groups: []\n",
	}))

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when content unchanged")
	}
}

func TestConfigMapSyncer_StaleFileRemoval(t *testing.T) {
	outputDir := t.TempDir()

	// A flat file left behind by an older config-reloader.
	staleFile := filepath.Join(outputDir, "old-shard__stale.yaml")
	if err := os.WriteFile(staleFile, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	syncer, _ := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"current.yaml": "groups: []\n",
	}))
	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Error("stale file was not removed")
	}
	if got := visibleFiles(t, outputDir); len(got) != 1 || got[0] != "current.yaml" {
		t.Errorf("unexpected visible files: %v", got)
	}
}

func TestConfigMapSyncer_ReplacesFlatFile(t *testing.T) {
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outputDir, "rules.yaml"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	syncer, _ := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"rules.yaml": "new",
	}))
	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Readlink(filepath.Join(outputDir, "rules.yaml")); err != nil {
		t.Errorf("expected a symlink: %v", err)
	}
	if got := readVisible(t, outputDir, "rules.yaml"); got != "new" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestConfigMapSyncer_MultipleConfigMaps(t *testing.T) {
	outputDir := t.TempDir()
	syncer, _ := newTestSyncer(t, outputDir,
		shardConfigMap("rules-generated-0", map[string]string{"rules1.yaml": "shard0-rules1"}),
		shardConfigMap("rules-generated-1", map[string]string{"rules2.yaml": "shard1-rules2"}),
	)

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	if got := visibleFiles(t, outputDir); len(got) != 2 {
		t.Errorf("expected 2 files, got %v", got)
	}
}

func TestConfigMapSyncer_ContentUpdateDetection(t *testing.T) {
	outputDir := t.TempDir()
	syncer, client := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"rules.yaml": "version: 1",
	}))

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	if _, err := client.CoreV1().ConfigMaps("gmp-system").Update(t.Context(),
		shardConfigMap("rules-generated-0", map[string]string{"rules.yaml": "version: 2"}),
		metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after content update")
	}
	if got := readVisible(t, outputDir, "rules.yaml"); got != "version: 2" {
		t.Errorf("expected updated content, got %q", got)
	}
	if dirs := payloadDirs(t, outputDir); len(dirs) != 1 {
		t.Errorf("expected old payload dir to be cleaned up, got %v", dirs)
	}
}

func TestConfigMapSyncer_MixedDataAndBinaryData(t *testing.T) {
	outputDir := t.TempDir()

	gzipContent := []byte{0x1f, 0x8b, 0x08, 0x00}
	cm := shardConfigMap("rules-generated-0", map[string]string{"uncompressed.yaml": "groups: []\n"})
	cm.BinaryData = map[string][]byte{"compressed.yaml": gzipContent}

	syncer, _ := newTestSyncer(t, outputDir, cm)
	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := visibleFiles(t, outputDir); len(got) != 2 {
		t.Fatalf("expected 2 files, got %v", got)
	}
	if got := readVisible(t, outputDir, "uncompressed.yaml"); got != "groups: []\n" {
		t.Errorf("unexpected text content: %q", got)
	}
	if got := readVisible(t, outputDir, "compressed.yaml"); got != string(gzipContent) {
		t.Errorf("unexpected binary content: %q", got)
	}
}

func TestConfigMapSyncer_SelectorFiltering(t *testing.T) {
	outputDir := t.TempDir()

	nonMatching := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-evaluator",
			Namespace: "gmp-system",
			Labels:    map[string]string{"app.kubernetes.io/name": "rule-evaluator"},
		},
		Data: map[string]string{"config.yaml": "should-not-appear"},
	}
	wrongNamespace := shardConfigMap("rules-generated-0", map[string]string{"rules.yaml": "wrong-namespace"})
	wrongNamespace.Namespace = "other-ns"

	syncer, _ := newTestSyncer(t, outputDir,
		shardConfigMap("rules-generated-0", map[string]string{"rules.yaml": "matched"}),
		nonMatching,
		wrongNamespace,
	)
	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := visibleFiles(t, outputDir); len(got) != 1 {
		t.Fatalf("expected 1 file, got %v", got)
	}
	if got := readVisible(t, outputDir, "rules.yaml"); got != "matched" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestConfigMapSyncer_ConfigMapRemoved(t *testing.T) {
	outputDir := t.TempDir()
	syncer, client := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"rules.yaml": "data",
	}))

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := visibleFiles(t, outputDir); len(got) != 1 {
		t.Fatalf("file should exist after first sync, got %v", got)
	}

	if err := client.CoreV1().ConfigMaps("gmp-system").Delete(t.Context(), "rules-generated-0", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true after ConfigMap deletion")
	}
	if got := visibleFiles(t, outputDir); len(got) != 0 {
		t.Errorf("expected 0 files after ConfigMap removed, got %v", got)
	}
}

func TestConfigMapSyncer_IgnoresUnexpectedNames(t *testing.T) {
	outputDir := t.TempDir()

	rogue := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-injected",
			Namespace: "gmp-system",
			Labels:    map[string]string{"monitoring.googleapis.com/rules-shard": "true"},
		},
		Data: map[string]string{"evil.yaml": "groups: [{name: evil, rules: []}]\n"},
	}

	syncer, _ := newTestSyncer(t, outputDir,
		shardConfigMap("rules-generated-0", map[string]string{"rules__default__test.yaml": "groups: []\n"}),
		rogue,
	)
	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	got := visibleFiles(t, outputDir)
	if len(got) != 1 || got[0] != "rules__default__test.yaml" {
		t.Errorf("expected only the shard file, got %v", got)
	}
}

func TestConfigMapSyncer_IgnoresInvalidKeys(t *testing.T) {
	outputDir := t.TempDir()
	syncer, _ := newTestSyncer(t, outputDir, shardConfigMap("rules-generated-0", map[string]string{
		"../escape.yaml": "escaped",
		"sub/dir.yaml":   "nested",
		"..data":         "clobber",
		"ok.yaml":        "fine",
	}))

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	got := visibleFiles(t, outputDir)
	if len(got) != 1 || got[0] != "ok.yaml" {
		t.Fatalf("expected only ok.yaml, got %v", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(outputDir), "escape.yaml")); !os.IsNotExist(err) {
		t.Error("file escaped the output dir")
	}
	target, err := os.Readlink(filepath.Join(outputDir, "..data"))
	if err != nil {
		t.Fatalf("..data was clobbered: %v", err)
	}
	if !strings.HasPrefix(target, "..") {
		t.Errorf("..data points at %q", target)
	}
}
