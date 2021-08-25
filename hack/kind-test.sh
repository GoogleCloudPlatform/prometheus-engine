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

# Docker registry auth (uses temporary token from gcloud credentials)
# source: https://github.com/kubernetes-sigs/kind/blob/4353f18abcfdfc45428e9231b4174ff9453f9d1c/site/static/examples/kind-gcr.sh
docker_registry_auth() {
  # desired cluster name; default is "kind"
  KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kind}"

  # create a temp file for the docker config
  echo "Creating temporary docker client config directory ..."
  DOCKER_CONFIG=$(mktemp -d)
  export DOCKER_CONFIG
  trap 'echo "Removing ${DOCKER_CONFIG}/*" && rm -rf ${DOCKER_CONFIG:?}' EXIT

  echo "Creating a temporary config.json"
  # This is to force the omission of credsStore, which is automatically
  # created on supported system. With credsStore missing, "docker login"
  # will store the password in the config.json file.
  # https://docs.docker.com/engine/reference/commandline/login/#credentials-store
  cat <<EOF >"${DOCKER_CONFIG}/config.json"
  {
   "auths": { "gcr.io": {} }
  }
EOF
  # login to gcr in DOCKER_CONFIG using an access token
  # https://cloud.google.com/container-registry/docs/advanced-authentication#access_token
  echo "Logging in to GCR in temporary docker client config directory ..."
  gcloud auth print-access-token | \
    docker login -u oauth2accesstoken --password-stdin https://gcr.io

  # setup credentials on each node
  echo "Moving credentials to kind cluster name='${KIND_CLUSTER_NAME}' nodes ..."
  for node in $(kind get nodes --name "${KIND_CLUSTER_NAME}"); do
    # the -oname format is kind/name (so node/name) we just want name
    node_name=${node#node/}
    # copy the config to where kubelet will look
    docker cp "${DOCKER_CONFIG}/config.json" "${node_name}:/var/lib/kubelet/config.json"
    # restart kubelet to pick up the config
    docker exec "${node_name}" systemctl restart kubelet.service
  done

  echo "Done!"
}

TMPDIR=$(mktemp -d)
SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

# Start port-forwarding to gcloud shell's docker daemon
# and port-forward the configured kind apiserver port 6443.
if docker ps; then
  echo ">>> docker daemon found"
else
  echo ">>> forwarding docker daemon from cloud-shell"
  gcloud alpha cloud-shell ssh -- -4 -nNT -L ${TMPDIR}/docker.sock:/var/run/docker.sock -L 6443:localhost:6443 &
  export DOCKER_HOST=unix://${TMPDIR}/docker.sock
  sleep 10
fi

# Idempotently create kind cluster
echo ">>> creating k8s cluster"
export KUBECONFIG=${TMPDIR}/.kube/config
kind delete cluster
kind create cluster --config=${SCRIPT_ROOT}/hack/kind-config.yaml

docker_registry_auth

# Need to ensure namespace is deployed first explicitly.
echo ">>> deploying static resources"
kubectl --context kind-kind apply -f ${SCRIPT_ROOT}/cmd/operator/deploy/operator/operator.yaml
kubectl --context kind-kind apply -f ${SCRIPT_ROOT}/cmd/operator/deploy/operator/
kubectl --context kind-kind apply -f ${SCRIPT_ROOT}/cmd/operator/deploy/ --recursive

echo ">>> executing gpe e2e tests"
go test -v ${SCRIPT_ROOT}/pkg/operator/e2e -args -project-id=test-proj -cluster=test-cluster -location=test-location
