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

# First argument is expected to be the name of the go test to run.
GO_TEST=$1

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
TEST_ARGS=""
# Convert kind cluster name to required regex if necessary.
# We need to ensure this name is not too long due to: https://github.com/kubernetes-sigs/kind/issues/623
# while still unique enough to avoid dups between similar test names when trimming.
# So we omit the Test* prefix and add a hash at the end.
KIND_CLUSTER_HASH=$(echo $RANDOM | md5sum | head -c4)
KIND_CLUSTER=$(echo ${GO_TEST#"Test"} | sed -r 's/[^[:alnum:]]//g' | sed -r 's/([A-Z])/-\L\1/g' | sed 's/^-//' | head -c28)
KIND_CLUSTER=${KIND_CLUSTER}-${KIND_CLUSTER_HASH}
KUBECTL="kubectl --context kind-${KIND_CLUSTER}"
# Ensure a unique label on any test data sent to GCM.
GMP_CLUSTER=$TAG_NAME
if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  PROJECT_ID=$(jq -r '.project_id' "${GOOGLE_APPLICATION_CREDENTIALS}")
else
  echo ">>> no credentials specified. running without GCM validation"
  TEST_ARGS="${TEST_ARGS} -skip-gcm"
fi
TEST_ARGS="${TEST_ARGS} -project-id=${PROJECT_ID} -location=${GMP_LOCATION} -cluster=${GMP_CLUSTER}"

create_kind_cluster() {
  echo ">>> creating kind cluster"
  cat <<EOF | kind create cluster --name ${KIND_CLUSTER} --config=-
  kind: Cluster
  apiVersion: kind.x-k8s.io/v1alpha4
  containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
EOF
}

add_registry_to_nodes() {
  echo ">>> adding registry to kind cluster nodes"
  REGISTRY_DIR="/etc/containerd/certs.d/localhost:${REGISTRY_PORT}"
  for node in $(kind get nodes --name ${KIND_CLUSTER}); do
    docker exec "${node}" mkdir -p "${REGISTRY_DIR}"
    cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${REGISTRY_DIR}/hosts.toml"
  [host."http://${REGISTRY_NAME}:5000"]
EOF
  done
}

connect_registry() {
  echo ">>> connecting registry to kind cluster"
  if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REGISTRY_NAME}")" = 'null' ]; then
    # Tolerate races of connecting registry container to kind network.
    docker network connect "kind" "${REGISTRY_NAME}" || true
  fi
}

document_registry() {
  echo ">>> documenting registry through configmap"
  cat <<EOF | $KUBECTL apply -f -
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: local-registry-hosting
    namespace: kube-public
  data:
    localRegistryHosting.v1: |
      host: "localhost:${REGISTRY_PORT}"
      help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

docker_tag_push() {
  for bin in "$@"; do
    REGISTRY_TAG=localhost:${REGISTRY_PORT}/${bin}:${TAG_NAME}
    echo ">>> tagging and pushing image: ${bin}"
    docker tag gmp/${bin} ${REGISTRY_TAG}
    docker push ${REGISTRY_TAG}
  done
}

# TODO(pintohutch): this is a bit hacky, but can be useful when testing.
# Ultimately this should be replaced with go templating.
update_manifests() {
  for bin in "$@"; do
    find manifests -type f -name "*.yaml" -exec sed -i "s#image: .*/${bin}:.*#image: localhost:${REGISTRY_PORT}/${bin}:${TAG_NAME}#g" {} \;
    if [ "$bin" = "go-synthetic" ]; then
      find examples/instrumentation -type f -name "*.yaml" -exec sed -i "s#image: .*/example-app:.*#image: localhost:${REGISTRY_PORT}/${bin}:${TAG_NAME}#g" {} \;
    fi
  done
}

# Set up local image registry and tag and push images to it.
# Finally update the install manifests to reference those images.
docker_tag_push $BINARIES
update_manifests $BINARIES

# Set up kind cluster and connect it to the local registry.
kind delete cluster --name ${KIND_CLUSTER}
create_kind_cluster
add_registry_to_nodes
connect_registry

# Run the go tests.
echo ">>> executing gmp e2e tests: ${GO_TEST}"
# Note: a test failure here should exit non-zero and leave the cluster running
# for debugging.
go test -v -timeout 10m "${REPO_ROOT}/e2e" -run "${GO_TEST:-.}" -args ${TEST_ARGS}

# Delete cluster if it's not set to clean up post-test.
# Otherwise, leave cluster running (e.g. for debugging).
if [ -z ${KIND_PERSIST+x} ]; then
  kind delete cluster --name ${KIND_CLUSTER}
fi
