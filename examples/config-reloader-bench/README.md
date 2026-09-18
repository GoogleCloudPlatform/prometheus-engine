# `config-reloader` E2E Memory Benchmark (`b/555908579`)

This directory contains an end-to-end memory benchmark harness comparing the **old (`v0.18.2-gke.1`, in-memory buffered)** vs. **new (`v0.19.2` / `main`, streaming `thanos-io/thanos@531d98a`)** `config-reloader` across:
1. **Default 4KB page size node pool** (`default-pool`, COS `amd64`, 4KB page size)
2. **Ubuntu `arm64-linux64k` 64KB page size node pool** (`ubuntu-64k-pool`, ARM64 `c4a-standard-2` / `t2a-standard-2`, 64KB page size kernel)

## Why This Setup?

- **Zero Prometheus Overhead / No Data Export**: Each simplified Collector Pod pairs `config-reloader` with a lightweight `prometheus-noop` container running `--export.disable` and a 2-line static config (`noop-prom.yaml`) on `127.0.0.1:19090`.
  - `config-reloader` still watches, gunzips, expands `${NODE_NAME}` env vars, hashes, and writes the full `/prometheus/config_out/config.yaml` (up to `8,500` jobs / `~33.7 MiB` uncompressed) and triggers `POST http://127.0.0.1:19090/-/reload`.
  - `prometheus-noop` immediately returns `200 OK` to `/-/ready` and `/-/reload` in `<1ms` without parsing 8,500 jobs or exporting any data to Cloud Monitoring.
- **Dual Scraping (Managed Prometheus + `prombenchy` in-cluster Prometheus)**:
  - [`monitoring.yaml`](monitoring.yaml) deploys a GMP `PodMonitoring` (scraping `:19091/metrics` on port `cfg-rel-ins`) and `ClusterNodeMonitoring` (scraping `/metrics/cadvisor` on all nodes for `container="config-reloader"`).
  - Because the pods carry `app: collector` and port `cfg-rel-ins`, `bwplotka-rw`'s in-cluster `core/prometheus-0` and `core/parca` also scrape and profile them automatically.

---

## Step 1: Add the Ubuntu `arm64-linux64k` (64KB Page Size) Node Pool to `bwplotka-rw`

Reuse cluster `bwplotka-rw` in `gpe-test-1` (`us-central1-a`), which already has `default-pool` (4KB page size COS) and Managed Prometheus enabled.

Add a 1-node ARM64 64KB page size node pool tainted with `bench-node=ubuntu-64k:NoSchedule` so only benchmark pods (and system DaemonSets) schedule on it:

```bash
# If gcloud hits a Context-Aware Access refresh error, export ADC token first:
gcloud auth application-default print-access-token > /tmp/adc_token.txt
export CLOUDSDK_AUTH_ACCESS_TOKEN_FILE=/tmp/adc_token.txt

# Option A: Ubuntu 24.04 1.36 arm64-linux64k (matches bwplotka-rw's 1.36.4 master version directly)
gcloud alpha container node-pools create ubuntu-64k-pool \
  --cluster=bwplotka-rw \
  --zone=us-central1-a \
  --project=gpe-test-1 \
  --machine-type=c4a-standard-2 \
  --num-nodes=1 \
  --image-type=CUSTOM_CONTAINERD \
  --image-project=ubuntu-os-gke-cloud \
  --image=ubuntu-gke-2404-1-36-arm64-v20260917-linux64k \
  --node-labels=bench-node=ubuntu-64k \
  --node-taints=bench-node=ubuntu-64k:NoSchedule

# Option B: Ubuntu 26.04 1.37 arm64-linux64k (Linux 7.0 64k page size; requires 1.37 cluster control plane)
# gcloud alpha container node-pools create ubuntu-64k-pool \
#   --cluster=bwplotka-rw \
#   --zone=us-central1-a \
#   --project=gpe-test-1 \
#   --machine-type=c4a-standard-2 \
#   --num-nodes=1 \
#   --image-type=CUSTOM_CONTAINERD \
#   --image-project=ubuntu-os-gke-cloud \
#   --image=ubuntu-gke-2604-1-37-arm64-v20260917-linux64k \
#   --node-labels=bench-node=ubuntu-64k \
#   --node-taints=bench-node=ubuntu-64k:NoSchedule
```

Verify page size (`65536` = 64KB) once the node is Ready:
```bash
kubectl run -it --rm check-pagesize --image=busybox --restart=Never \
  --overrides='{"spec":{"nodeSelector":{"bench-node":"ubuntu-64k"},"tolerations":[{"key":"bench-node","value":"ubuntu-64k","effect":"NoSchedule"}]}}' \
  -- getconf PAGESIZE
```

---

## Step 2: Deploy Simplified Collector Pods & Monitoring

```bash
kubectl apply -f examples/config-reloader-bench/configmap-example.yaml
kubectl apply -f examples/config-reloader-bench/collector-default-node.yaml
kubectl apply -f examples/config-reloader-bench/collector-ubuntu64k-node.yaml
kubectl apply -f examples/config-reloader-bench/monitoring.yaml
```

This creates 5 pods in `config-reloader-bench`:
| Pod Name | Node Pool (Page Size) | `config-reloader` Image | Memory Limit | Purpose |
| :--- | :--- | :--- | :--- | :--- |
| `collector-default-018` | `default-pool` (4KB) | `v0.18.2-gke.1` (buffered) | `256Mi` | Baseline 0.18 RSS on 4KB page kernel |
| `collector-default-new` | `default-pool` (4KB) | `v0.19.2-streaming` | `256Mi` | Streaming RSS on 4KB page kernel |
| `collector-ubuntu64k-018` | `ubuntu-64k-pool` (64KB) | `v0.18.2-gke.1` (buffered) | `256Mi` | Uncapped 0.18 RSS on 64KB page kernel |
| `collector-ubuntu64k-new` | `ubuntu-64k-pool` (64KB) | `v0.19.2-streaming` | `256Mi` | Uncapped streaming RSS on 64KB page kernel |
| `collector-ubuntu64k-new-32mi` | `ubuntu-64k-pool` (64KB) | `v0.19.2-streaming` | `32Mi` | Tests whether `32Mi` limit (PR `#2297`) OOMKills on 64KB pages |

---

## Step 3: Test Different Config Sizes Live

Use [`gen_configmap.go`](gen_configmap.go) to update `ConfigMap/collector-bench-config` in-place. All pods share this ConfigMap and will automatically reload within ~10–30 seconds:

```bash
# 1. Small config (10 jobs, ~40 KiB uncompressed)
go run ./examples/config-reloader-bench/gen_configmap.go -jobs=10 | kubectl apply -f -

# 2. Medium config (1,000 jobs, ~4 MiB uncompressed)
go run ./examples/config-reloader-bench/gen_configmap.go -jobs=1000 | kubectl apply -f -

# 3. Max ConfigMap limit (8,500 jobs, ~960 KiB gzipped -> ~33.7 MiB uncompressed)
go run ./examples/config-reloader-bench/gen_configmap.go -jobs=8500 | kubectl apply -f -
```

---

## Step 4: PromQL Queries to Compare Memory Usage

You can run these in Cloud Monitoring (Metrics Explorer -> PromQL) or by port-forwarding `bwplotka-rw`'s in-cluster Prometheus (`kubectl -n core port-forward svc/prometheus 9090:9090`):

1. **Kernel cAdvisor Working Set & RSS (`container_memory_working_set_bytes` / `container_memory_rss`)**:
   ```promql
   max by (pod, node) (
     container_memory_working_set_bytes{namespace="config-reloader-bench", container="config-reloader"}
   ) / 1024 / 1024
   ```
   ```promql
   max by (pod, node) (
     container_memory_rss{namespace="config-reloader-bench", container="config-reloader"}
   ) / 1024 / 1024
   ```

2. **Peak cAdvisor Memory Usage (`container_memory_max_usage_bytes`)**:
   ```promql
   max by (pod, node) (
     container_memory_max_usage_bytes{namespace="config-reloader-bench", container="config-reloader"}
   ) / 1024 / 1024
   ```

3. **Go Runtime Heap & Process Resident Memory (from `:19091/metrics`)**:
   ```promql
   max by (pod, reloader_version, node_type) (
     process_resident_memory_bytes{namespace="config-reloader-bench"}
   ) / 1024 / 1024
   ```
   ```promql
   max by (pod, reloader_version, node_type) (
     go_memstats_sys_bytes{namespace="config-reloader-bench"}
   ) / 1024 / 1024
   ```

4. **OOMKill Events (`container_oom_events_total`)**:
   ```promql
   sum by (namespace, pod, node) (
     container_oom_events_total{namespace=~"config-reloader-bench|gmp-system", container="config-reloader"}
   )
   ```
