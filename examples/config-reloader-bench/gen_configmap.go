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

// Command gen_configmap outputs a Kubernetes ConfigMap YAML containing a
// realistic gzipped GMP Prometheus configuration with -jobs=N scrape jobs.
//
// Usage:
//
//	go run ./examples/config-reloader-bench/gen_configmap.go -jobs=8500 | kubectl apply -f -
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
)

func main() {
	jobs := flag.Int("jobs", 1000, "Number of realistic PodMonitoring scrape jobs to generate (8500 ~= 960 KiB gzipped / 33.7 MiB uncompressed)")
	namespace := flag.String("namespace", "config-reloader-bench", "Target Kubernetes namespace")
	name := flag.String("name", "collector-bench-config", "Target ConfigMap name")
	flag.Parse()

	var raw bytes.Buffer
	raw.WriteString(`global:
  scrape_interval: 30s
  scrape_timeout: 10s
  evaluation_interval: 30s
  external_labels:
    node: "${NODE_NAME}"
scrape_configs:
`)

	for i := 0; i < *jobs; i++ {
		fmt.Fprintf(&raw, `- job_name: PodMonitoring/ns-%d/app-monitoring-%d/http-metrics-%d
  honor_labels: false
  honor_timestamps: true
  scrape_interval: 30s
  scrape_timeout: 10s
  metrics_path: /metrics
  scheme: http
  follow_redirects: true
  enable_http2: true
  kubernetes_sd_configs:
  - role: pod
    kubeconfig_file: ""
    follow_redirects: true
    enable_http2: true
    selectors:
    - role: pod
      field: spec.nodeName=${NODE_NAME}
    namespaces:
      own_namespace: false
      names:
      - namespace-workload-%d
  relabel_configs:
  - source_labels: [__meta_kubernetes_pod_phase]
    separator: ;
    regex: (Failed|Succeeded)
    replacement: $1
    action: drop
  - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_name]
    separator: ;
    regex: app-instance-%d
    replacement: $1
    action: keep
  - source_labels: [__meta_kubernetes_namespace]
    separator: ;
    regex: (.*)
    target_label: namespace
    replacement: $1
    action: replace
  - source_labels: [__meta_kubernetes_pod_name]
    separator: ;
    regex: (.*)
    target_label: pod
    replacement: $1
    action: replace
  - source_labels: [__meta_kubernetes_pod_container_name]
    separator: ;
    regex: (.*)
    target_label: container
    replacement: $1
    action: replace
  - source_labels: [__meta_kubernetes_pod_name, __meta_kubernetes_pod_container_name, __meta_kubernetes_pod_container_port_name]
    separator: ;
    regex: (.+);(.+);(http-metrics-%d)
    target_label: instance
    replacement: $1:$2:$3
    action: replace
  - separator: ;
    regex: (.*)
    target_label: job
    replacement: app-monitoring-%d
    action: replace
  - source_labels: [__meta_kubernetes_pod_ip]
    separator: ;
    regex: (.+)
    target_label: __address__
    replacement: $1:8080
    action: replace
  metric_relabel_configs:
  - source_labels: [__name__]
    separator: ;
    regex: (go_gc_duration_seconds.*|go_goroutines|go_memstats_alloc_bytes|http_requests_total_%d)
    replacement: $1
    action: keep
  - source_labels: [env]
    separator: ;
    regex: (staging|production-%d)
    target_label: environment
    replacement: $1
    action: replace
`, i%100, i, i%5, i%100, i, i%5, i, i, i%10)
	}

	var gzBuf bytes.Buffer
	gzWriter, err := gzip.NewWriterLevel(&gzBuf, gzip.BestCompression)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create gzip writer: %v\n", err)
		os.Exit(1)
	}
	if _, err := gzWriter.Write(raw.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to compress config: %v\n", err)
		os.Exit(1)
	}
	if err := gzWriter.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to close gzip writer: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Generated config with %d jobs: uncompressed=%d KiB, gzipped=%d KiB\n",
		*jobs, raw.Len()/1024, gzBuf.Len()/1024)

	encoded := base64.StdEncoding.EncodeToString(gzBuf.Bytes())

	fmt.Printf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  noop-prom.yaml: |
    global:
      scrape_interval: 60s
binaryData:
  config.yaml.gz: %s
`, *name, *namespace, encoded)
}
