#!/usr/bin/env bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# NOTE: examples/inject-gmp-sidecar.sh is generated from
# charts/inject-gmp-sidecar/inject-gmp-sidecar.sh.tpl by `make regen`.

# Example script that injects a dedicated GMP collector into an existing workload. It adds
# Prometheus (GMP fork) and config-reloader sidecars, which scrape the workload's metrics
# endpoint from within the pod and export the metrics to Google Cloud Monitoring.
#
# The script:
#   1. Reads the cluster, location and project_id external labels from the managed
#      collection configuration (gmp-system/collector ConfigMap).
#   2. Creates or updates the ${NAMESPACE}/${NAME} ConfigMap with a Prometheus configuration
#      that scrapes localhost:${PORT}${METRICS_PATH}.
#   3. Patches the workload with a strategic merge patch that adds the config-init init
#      container, the prometheus and config-reloader containers and their volumes.
#
# Prerequisites:
#   * Managed collection enabled in the cluster (e.g. on GKE).
#   * kubectl configured for the cluster and yq v4 (https://github.com/mikefarah/yq).
#   * Workload pods can write metrics to Cloud Monitoring (roles/monitoring.metricWriter),
#     e.g. using Workload Identity Federation for GKE.
#
# Usage: edit the variables below to match your workload and run the script.

set -exuo pipefail

KIND="deployment"
NAMESPACE="default"
NAME="example-deployment"
CONTAINER="example-service"
METRICS_PATH="/metrics"
PORT=80
# Optional extra flags for the Prometheus sidecar, e.g. "--export.disable" when running
# without Google Cloud credentials.
PROMETHEUS_EXTRA_ARGS="${PROMETHEUS_EXTRA_ARGS:-}"

if ! command -v yq &> /dev/null; then
  echo "Error: yq is not installed. Please install it (e.g., via 'go install github.com/mikefarah/yq/v4@latest' or from https://github.com/mikefarah/yq)." >&2
  exit 1
fi

# Extract labels. Missing labels are printed as empty strings rather than "null".
CLUSTER_NAME=$(kubectl -n gmp-system get configmap/collector -o jsonpath='{.data.config\.yaml}' | yq '.global.external_labels.cluster // ""')
LOCATION=$(kubectl -n gmp-system get configmap/collector -o jsonpath='{.data.config\.yaml}' | yq '.global.external_labels.location // ""')
PROJECT_ID=$(kubectl -n gmp-system get configmap/collector -o jsonpath='{.data.config\.yaml}' | yq '.global.external_labels.project_id // ""')

if [[ -z "${CLUSTER_NAME}" || -z "${LOCATION}" || -z "${PROJECT_ID}" ]]; then
  echo "Error: Failed to extract cluster, location, or project_id from gmp-system/collector configmap." >&2
  exit 1
fi

# Images come from charts/values.global.yaml, so they are updated together with our manifests.
DISTROLESS_IMAGE=gke.gcr.io/gke-distroless/bash:gke_distroless_20260815.00_p0
PROMETHEUS_IMAGE=gke.gcr.io/prometheus-engine/prometheus:v3.13.0-gmp.1-gke.0@sha256:b8e81d2737a3ca7126b06cbd4bb495b4f9c694e9431e5a280d7d37f289ff2708
CONFIG_RELOADER_IMAGE=gke.gcr.io/prometheus-engine/config-reloader:v0.17.3-gke.0

# Define the scrape configuration.
CONFIG_MAP=$(
	cat <<INNER_EOF
global:
  scrape_interval: 30s
  # Keep Prometheus 2.x metric name validation, same as managed collection.
  metric_name_validation_scheme: legacy
  metric_name_escaping_scheme: underscores
  external_labels:
    cluster: ${CLUSTER_NAME}
    location: ${LOCATION}
    project_id: ${PROJECT_ID}
scrape_configs:
- job_name: DedicatedCollector/${NAME}
  metrics_path: ${METRICS_PATH}
  static_configs:
  - targets: ['localhost:${PORT}']
    labels:
      # Common GMP labels.
      container: ${CONTAINER}
      node: \$(NODE_NAME)
      pod: \$(POD_NAME)
      top_level_controller_name: ${NAME}
      top_level_controller_type: ${KIND}
INNER_EOF
)

kubectl -n "${NAMESPACE}" create configmap "${NAME}" --from-literal="config.yaml=${CONFIG_MAP}" --dry-run=client -o yaml | kubectl apply -f -

# Construct Strategic Merge Patch.
STRATEGIC_PATCH=$(
	cat <<INNER_EOF
spec:
  template:
    spec:
      volumes:
      - name: config
        configMap:
          name: ${NAME}
      - name: prometheus-db
        emptyDir: {}
      - name: config-out
        emptyDir: {}
      initContainers:
      - name: config-init
        image: ${DISTROLESS_IMAGE}
        command: ["/bin/bash", "-c", ": > /prometheus/config_out/config.yaml"]
        volumeMounts:
        - name: config-out
          mountPath: /prometheus/config_out
      containers:
      - name: prometheus
        image: ${PROMETHEUS_IMAGE}
        args:
        - --config.file=/prometheus/config_out/config.yaml
        - --storage.tsdb.path=/prometheus/data
        - --storage.tsdb.retention.time=24h
        - --web.enable-lifecycle
        - --storage.tsdb.no-lockfile
        - --web.route-prefix=/
        - --log.level=debug
        env:
        # Additional flags, parsed by the GMP Prometheus fork.
        - name: EXTRA_ARGS
          value: "${PROMETHEUS_EXTRA_ARGS}"
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: config-out
          mountPath: /prometheus/config_out
          readOnly: true
        - name: prometheus-db
          mountPath: /prometheus/data
      - name: config-reloader
        image: ${CONFIG_RELOADER_IMAGE}
        args:
        - --config-file=/prometheus/config/config.yaml
        - --config-file-output=/prometheus/config_out/config.yaml
        - --reload-url=http://localhost:9090/-/reload
        - --ready-url=http://localhost:9090/-/ready
        - --listen-address=:19091
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        volumeMounts:
        - name: config
          mountPath: /prometheus/config
        - name: config-out
          mountPath: /prometheus/config_out
INNER_EOF
)

# Apply the Patch
kubectl -n "${NAMESPACE}" patch "${KIND}" "${NAME}" --patch "${STRATEGIC_PATCH}"
