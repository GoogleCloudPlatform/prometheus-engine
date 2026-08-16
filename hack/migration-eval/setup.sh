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

# Strictly require local Kind cluster to prevent any modification to user's active kubectl context
if ! command -v kind >/dev/null 2>&1; then
  echo "❌ ERROR: 'kind' CLI is not installed."
  echo "This evaluation harness runs strictly within an isolated local Kind cluster ('gmp-eval')."
  echo "Please install kind (https://kind.sigs.k8s.io/) before running this script."
  exit 1
fi

echo "=== 1. Building and installing 'gmp-migrate' CLI ==="
mkdir -p "${REPO_ROOT}/bin"
go build -o "${REPO_ROOT}/bin/gmp-migrate" "${REPO_ROOT}/cmd/gmp-migrate"

echo "=== 2. Creating/Verifying local Kind cluster 'gmp-eval' ==="
if kind get clusters 2>/dev/null | grep -q "^gmp-eval$"; then
  echo "Found existing Kind cluster 'gmp-eval'."
else
  echo "Creating local Kind cluster 'gmp-eval'..."
  kind create cluster --name gmp-eval
fi

KUBECTL_CMD="kubectl --context kind-gmp-eval"

echo "=== 3. Installing in-repo GMP CRDs ==="
${KUBECTL_CMD} apply -f "${REPO_ROOT}/manifests/setup.yaml"

echo "=== 4. Installing upstream Prometheus Operator CRDs ==="
${KUBECTL_CMD} apply -f https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.79.2/stripped-down-crds.yaml

echo "=== 5. Deploying isolated evaluation namespaces and workloads ==="
${KUBECTL_CMD} apply -f "${SCRIPT_DIR}/workloads.yaml"

echo "=== Evaluation environment setup complete in isolated Kind cluster 'gmp-eval'! ==="
echo "You can now run evaluations using monitors in: ${SCRIPT_DIR}/monitors/"
