#!/usr/bin/env bash
# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "=== 1. Checking Cluster Environment ==="
if command -v kind >/dev/null 2>&1; then
  if kind get clusters 2>/dev/null | grep -q "^gmp-eval$"; then
    echo "Found existing Kind cluster 'gmp-eval'."
  else
    echo "Creating local Kind cluster 'gmp-eval'..."
    kind create cluster --name gmp-eval
  fi
else
  CURRENT_CTX="$(kubectl config current-context 2>/dev/null || echo 'none')"
  echo "'kind' CLI not found. Using active kubectl context: ${CURRENT_CTX}"
fi

echo "=== 2. Installing in-repo GMP CRDs ==="
kubectl apply -f "${REPO_ROOT}/manifests/setup.yaml"

echo "=== 3. Installing upstream Prometheus Operator CRDs ==="
kubectl apply -f https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.79.2/stripped-down-crds.yaml

echo "=== 4. Creating 'eval-apps' namespace and deploying test workloads ==="
kubectl create namespace eval-apps --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "${SCRIPT_DIR}/workloads.yaml"

echo "=== Evaluation environment setup complete! ==="
echo "You can now run evaluations using monitors in: ${SCRIPT_DIR}/monitors/"
