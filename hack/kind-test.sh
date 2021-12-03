#!/usr/bin/env bash

# Copyright 2021 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

TMPDIR=$(mktemp -d)
SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

# Start port-forwarding to gcloud shell's docker daemon
# and port-forward the configured kind apiserver port 6443.
if docker ps; then
  echo ">>> docker daemon found"
else
  echo ">>> forwarding docker daemon from cloud-shell"
  gcloud alpha cloud-shell ssh -- -4 -nNT -L ${TMPDIR}/docker.sock:/var/run/docker.sock -L 6443:localhost:6443 &
  # Wait for cloud-shell to mount Docker unix socket.
  while [ ! -f $HOME/.ssh/known_hosts ]; do echo ">>> waiting for cloud-shell docker socket mount"; sleep 2; done
  echo ">>> mounted docker socket at ${TMPDIR}/docker.sock"
  export DOCKER_HOST=unix://${TMPDIR}/docker.sock
  sleep 2
fi

# Idempotently create kind cluster
export KUBECONFIG=${TMPDIR}/.kube/config
echo ">>> creating k8s cluster using KUBECONFIG at ${KUBECONFIG}"
kind delete cluster
kind create cluster --config=${SCRIPT_ROOT}/hack/kind-config.yaml

# Need to ensure namespace is deployed first explicitly.
echo ">>> deploying static resources"
kubectl --context kind-kind apply -f ${SCRIPT_ROOT}/examples/setup.yaml
kubectl --context kind-kind apply -f ${SCRIPT_ROOT}/examples/operator.yaml
kubectl --context kind-kind apply -f ${SCRIPT_ROOT}/examples/rule-evaluator.yaml
kubectl --context kind-kind apply -f ${SCRIPT_ROOT}/cmd/operator/deploy/ --recursive

echo ">>> executing gmp e2e tests"
go test -v ${SCRIPT_ROOT}/pkg/operator/e2e -args -project-id=test-proj -cluster=test-cluster -location=test-loc -skip-gcm
