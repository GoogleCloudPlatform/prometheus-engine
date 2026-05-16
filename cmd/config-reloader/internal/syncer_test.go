// Copyright 2024 Google LLC
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
	"testing"
	"time"

	"github.com/go-kit/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestConfigMapSyncer_BasicSync(t *testing.T) {
	outputDir := t.TempDir()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels: map[string]string{
				"monitoring.googleapis.com/rules-shard": "true",
			},
		},
		Data: map[string]string{
			"rules__default__test.yaml": "groups:\n- name: test\n  rules: []\n",
		},
	}

	client := fake.NewSimpleClientset(cm)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on first sync")
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "rules-generated-0__rules__default__test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "groups:\n- name: test\n  rules: []\n" {
		t.Errorf("unexpected file content: %q", data)
	}
}

func TestConfigMapSyncer_NoChangeOnSecondSync(t *testing.T) {
	outputDir := t.TempDir()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels: map[string]string{
				"monitoring.googleapis.com/rules-shard": "true",
			},
		},
		Data: map[string]string{
			"test.yaml": "groups: []\n",
		},
	}

	client := fake.NewSimpleClientset(cm)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

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

	staleFile := filepath.Join(outputDir, "old-shard__stale.yaml")
	if err := os.WriteFile(staleFile, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels: map[string]string{
				"monitoring.googleapis.com/rules-shard": "true",
			},
		},
		Data: map[string]string{
			"current.yaml": "groups: []\n",
		},
	}

	client := fake.NewSimpleClientset(cm)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Error("stale file was not removed")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "rules-generated-0__current.yaml")); err != nil {
		t.Errorf("current file missing: %v", err)
	}
}

func TestConfigMapSyncer_MultipleConfigMaps(t *testing.T) {
	outputDir := t.TempDir()

	cm0 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels: map[string]string{
				"monitoring.googleapis.com/rules-shard": "true",
			},
		},
		Data: map[string]string{"rules1.yaml": "shard0-rules1"},
	}
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-1",
			Namespace: "gmp-system",
			Labels: map[string]string{
				"monitoring.googleapis.com/rules-shard": "true",
			},
		},
		Data: map[string]string{"rules2.yaml": "shard1-rules2"},
	}

	client := fake.NewSimpleClientset(cm0, cm1)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(entries))
	}
}

func TestConfigMapSyncer_ContentUpdateDetection(t *testing.T) {
	outputDir := t.TempDir()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels: map[string]string{
				"monitoring.googleapis.com/rules-shard": "true",
			},
		},
		Data: map[string]string{"rules.yaml": "version: 1"},
	}

	client := fake.NewSimpleClientset(cm)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	cm.Data["rules.yaml"] = "version: 2"
	if _, err := client.CoreV1().ConfigMaps("gmp-system").Update(t.Context(), cm, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	changed, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after content update")
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "rules-generated-0__rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "version: 2" {
		t.Errorf("expected updated content, got %q", data)
	}
}

func TestConfigMapSyncer_MixedDataAndBinaryData(t *testing.T) {
	outputDir := t.TempDir()

	gzipContent := []byte{0x1f, 0x8b, 0x08, 0x00}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels: map[string]string{
				"monitoring.googleapis.com/rules-shard": "true",
			},
		},
		Data:       map[string]string{"uncompressed.yaml": "groups: []\n"},
		BinaryData: map[string][]byte{"compressed.yaml": gzipContent},
	}

	client := fake.NewSimpleClientset(cm)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files, got %d", len(entries))
	}

	textData, err := os.ReadFile(filepath.Join(outputDir, "rules-generated-0__uncompressed.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(textData) != "groups: []\n" {
		t.Errorf("unexpected text content: %q", textData)
	}

	binData, err := os.ReadFile(filepath.Join(outputDir, "rules-generated-0__compressed.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(binData) != len(gzipContent) {
		t.Errorf("expected %d binary bytes, got %d", len(gzipContent), len(binData))
	}
}

func TestConfigMapSyncer_SelectorFiltering(t *testing.T) {
	outputDir := t.TempDir()

	matching := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels:    map[string]string{"monitoring.googleapis.com/rules-shard": "true"},
		},
		Data: map[string]string{"rules.yaml": "matched"},
	}
	nonMatching := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-evaluator",
			Namespace: "gmp-system",
			Labels:    map[string]string{"app.kubernetes.io/name": "rule-evaluator"},
		},
		Data: map[string]string{"config.yaml": "should-not-appear"},
	}
	wrongNamespace := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "other-ns",
			Labels:    map[string]string{"monitoring.googleapis.com/rules-shard": "true"},
		},
		Data: map[string]string{"rules.yaml": "wrong-namespace"},
	}

	client := fake.NewSimpleClientset(matching, nonMatching, wrongNamespace)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "rules-generated-0__rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "matched" {
		t.Errorf("unexpected content: %q", data)
	}
}

func TestConfigMapSyncer_ConfigMapRemoved(t *testing.T) {
	outputDir := t.TempDir()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rules-generated-0",
			Namespace: "gmp-system",
			Labels:    map[string]string{"monitoring.googleapis.com/rules-shard": "true"},
		},
		Data: map[string]string{"rules.yaml": "data"},
	}

	client := fake.NewSimpleClientset(cm)
	syncer := newConfigMapSyncerWithClient(client, "gmp-system", "monitoring.googleapis.com/rules-shard=true", outputDir, time.Second, log.NewNopLogger())

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "rules-generated-0__rules.yaml")); err != nil {
		t.Fatal("file should exist after first sync")
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

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 files after ConfigMap removed, got %d", len(entries))
	}
}
