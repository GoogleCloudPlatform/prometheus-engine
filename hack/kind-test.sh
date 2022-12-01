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

function main() {
  local script_root=$(dirname "${BASH_SOURCE[0]}")/..
  kind delete cluster

  # See https://kind.sigs.k8s.io/docs/user/local-registry/
  # Create registry container unless it already exists
  local reg_name='kind-registry'
  local reg_port='5001'
  if [ "$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)" != 'true' ]; then
    docker run \
      -d --restart=always -p "127.0.0.1:${reg_port}:5000" --name "${reg_name}" \
      registry:2
  fi

  # create a cluster with the local registry enabled in containerd
  cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_name}:5000"]
EOF

  # connect the registry to the cluster network if not already connected
  if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${reg_name}")" = 'null' ]; then
    docker network connect "kind" "${reg_name}"
  fi

  # Document the local registry
  # https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

  # Need to ensure namespace is deployed first explicitly.
  echo ">>> deploying static resources"

  local project_id="test-proj"
  local cluster="test-cluster"
  local location="test-loc"

  local deployment_dir="${script_root}/build/kindtest/overlays"
  mkdir -p "${deployment_dir}"
  cat > "${deployment_dir}/kustomization.yaml" <<- EOM
# For creating local kind builds on port 5000.
resources:
  - ../base/operator

patches:
- patch: |-
    - op: add
      path: /spec/template/spec/containers/0/args/-
      value: --project-id=${project_id}
    - op: add
      path: /spec/template/spec/containers/0/args/-
      value: --cluster=${cluster}
    - op: add
      path: /spec/template/spec/containers/0/args/-
      value: --location=${location}
  target:
    kind: Deployment
    name: gmp-operator
    namespace: gmp-system
EOM

  kubectl --context kind-kind apply -f "${script_root}/manifests/setup.yaml"
  kubectl --context kind-kind apply -k "${deployment_dir}"

  echo ">>> executing gmp e2e tests"
  go test -v "${script_root}/pkg/operator/e2e" -args -project-id="${project_id}" -cluster="${cluster}" -location="${location}" -skip-gcm
}

main
