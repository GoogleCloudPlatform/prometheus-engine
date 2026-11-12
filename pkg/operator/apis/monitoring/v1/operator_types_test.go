// Copyright 2022 Google LLC
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

package v1

import (
	"log/slog"
	"testing"

	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"go.yaml.in/yaml/v4"
)

func TestCollectionSpec_ScrapeConfigs(t *testing.T) {
	spec := CollectionSpec{
		KubeletScraping: &KubeletScraping{
			Interval: "15s",
		},
	}
	sc, err := spec.ScrapeConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(sc) != 2 {
		t.Fatalf("expected 2 kubelet scrape configs, got %d", len(sc))
	}
	// Verify that marshaling with yaml.v3 and loading with promconfig.Load succeeds,
	// and that the unmarshaled relabel configurations properly populate the expected target labels.
	promCfg := promconfig.Config{
		ScrapeConfigs: sc,
	}
	out, err := yaml.Marshal(promCfg)
	if err != nil {
		t.Fatalf("marshaling scrape config: %s", err)
	}

	loaded, err := promconfig.Load(string(out), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("loading scrape config: %s", err)
	}

	for _, cfg := range loaded.ScrapeConfigs {
		if got, want := string(cfg.ScrapeFallbackProtocol), "PrometheusText0.0.4"; got != want {
			t.Errorf("job %q: unexpected ScrapeFallbackProtocol: got %q, want %q", cfg.JobName, got, want)
		}

		// Verify target relabeling behavior on node metadata.
		input := labels.FromMap(map[string]string{
			"__meta_kubernetes_node_name": "test-node",
		})
		lb := labels.NewBuilder(input)
		for _, rc := range cfg.RelabelConfigs {
			relabel.ProcessBuilder(lb, rc)
		}
		res := lb.Labels()
		if got, want := res.Get("job"), "kubelet"; got != want {
			t.Errorf("job %q: unexpected job label: got %q, want %q", cfg.JobName, got, want)
		}
		if got, want := res.Get("node"), "test-node"; got != want {
			t.Errorf("job %q: unexpected node label: got %q, want %q", cfg.JobName, got, want)
		}
		wantInstance := "test-node:metrics"
		if cfg.JobName == "kubelet/cadvisor" {
			wantInstance = "test-node:cadvisor"
		}
		if got, want := res.Get("instance"), wantInstance; got != want {
			t.Errorf("job %q: unexpected instance label: got %q, want %q", cfg.JobName, got, want)
		}
	}
}
