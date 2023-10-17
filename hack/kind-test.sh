#!/usr/bin/env bash

# Copyright 2022 Google LLC
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

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

kind delete cluster

# We expect kind create to run "kind export kubeconfig" for us, which should
# export 'kind kind' kubernetes context referenced later.
kind create cluster

# Need to ensure namespace is deployed first explicitly.
echo ">>> deploying static resources"
kubectl --context kind-kind apply -f "${REPO_ROOT}/manifests/setup.yaml"

# TODO(pintohutch): find a way to incorporate webhooks back into our kind tests.
# This is a workaround for now.
for m in `ls -d ${REPO_ROOT}/cmd/operator/deploy/operator/* | grep -v webhook`
do
  kubectl --context kind-kind apply -f "${m}"
done
kubectl --context kind-kind apply -f "${REPO_ROOT}/manifests/rule-evaluator.yaml"

echo ">>> executing gmp e2e tests"
# We don't cleanup resources because this script recreates the cluster.
go test -v "${REPO_ROOT}/e2e" -run "${TEST_RUN:-.}" -args -project-id=test-proj -cluster=test-cluster -location=test-loc -skip-gcm=true -cleanup-resources=false ${TEST_ARGS}
