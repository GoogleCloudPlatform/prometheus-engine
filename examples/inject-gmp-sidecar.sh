#!/usr/bin/env bash
# Copyright 2024 Google LLC
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

set -exuo pipefail

KIND="deployment"
NAMESPACE="default"
NAME="example-deployment"
CONTAINER="example-service"
METRICS_PATH="/metrics"
PORT=80

if ! command -v yq &> /dev/null; then
  echo "Error: yq is not installed. Please install it (e.g., via 'go install github.com/mikefarah/yq/v4@latest' or from https://github.com/mikefarah/yq)." >&2
  exit 1
fi

# Extract labels.
CLUSTER_NAME=$(kubectl -n gmp-system get configmap/collector -o jsonpath='{.data.config\.yaml}' | yq '.global.external_labels.cluster')
LOCATION=$(kubectl -n gmp-system get configmap/collector -o jsonpath='{.data.config\.yaml}' | yq '.global.external_labels.location')
PROJECT_ID=$(kubectl -n gmp-system get configmap/collector -o jsonpath='{.data.config\.yaml}' | yq '.global.external_labels.project_id')


if [[ -z "${CLUSTER_NAME}" || -z "${LOCATION}" || -z "${PROJECT_ID}" ]]; then
  echo "Error: Failed to extract cluster, location, or project_id from gmp-system/collector configmap." >&2
  exit 1
fi

# Images need to be updated periodically.
DISTROLESS_IMAGE=gke.gcr.io/gke-distroless/bash:gke_distroless_20240220.00_p0@sha256:828371616edc2c38e36868e2f8c992df37e484df72670f148de59867dfdd2490
PROMETHEUS_IMAGE=gke.gcr.io/prometheus-engine/prometheus:v2.53.5-gmp.4-gke.0@sha256:6f349dc0be36c8a61be183254f1126c9935f5332daa96c481f7e0e1b20fe0513
CONFIG_RELOADER_IMAGE=gke.gcr.io/prometheus-engine/config-reloader:v0.18.0-gke.2@sha256:b41862ee7ee3e9f24112ccdb0e53060085af1a8347054a7dbcff04467d3e1e9c

# Define the scrape configuration.
CONFIG_MAP=$(
	cat <<INNER_EOF
global:
  scrape_interval: 30s
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
