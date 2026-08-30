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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/thanos/pkg/reloader"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The layout works only because the Thanos reloader follows the file symlinks
// and skips the payload dir behind them. Pin that against a dependency bump.
func TestSyncedDirIsReadableByThanosReloader(t *testing.T) {
	in, out := t.TempDir(), t.TempDir()
	syncer, client := newTestSyncer(t, in, shardConfigMap("rules-generated-0", map[string]string{
		"rules__default__a.yaml": "groups: [] # a",
		"rules__default__b.yaml": "groups: [] # b",
	}))
	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	reloads := make(chan struct{}, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reloads <- struct{}{}
	}))
	defer srv.Close()
	reloadURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	rel := reloader.New(log.NewNopLogger(), prometheus.NewRegistry(), &reloader.Options{
		ReloadURL:     reloadURL,
		CfgDirs:       []reloader.CfgDirOption{{Dir: in, OutputDir: out}},
		WatchInterval: 200 * time.Millisecond,
		RetryInterval: 100 * time.Millisecond,
		DelayInterval: time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	go func() { _ = rel.Watch(ctx) }()

	awaitReload(ctx, t, reloads)
	if got := visibleFiles(t, out); len(got) != 2 || got[0] != "rules__default__a.yaml" || got[1] != "rules__default__b.yaml" {
		t.Fatalf("unexpected output dir contents: %v", got)
	}
	if got := readVisible(t, out, "rules__default__a.yaml"); got != "groups: [] # a" {
		t.Fatalf("unexpected content: %q", got)
	}

	if _, err := client.CoreV1().ConfigMaps("gmp-system").Update(t.Context(),
		shardConfigMap("rules-generated-0", map[string]string{
			"rules__default__a.yaml": "groups: [] # a2",
			"rules__default__b.yaml": "groups: [] # b",
		}), metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		awaitReload(ctx, t, reloads)
		data, err := os.ReadFile(filepath.Join(out, "rules__default__a.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) == "groups: [] # a2" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("updated content never propagated, got %q", data)
		}
	}
}

func awaitReload(ctx context.Context, t *testing.T, reloads <-chan struct{}) {
	t.Helper()
	select {
	case <-reloads:
	case <-ctx.Done():
		t.Fatal("no reload triggered")
	}
}
