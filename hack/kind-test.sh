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
TAG_NAME=$(date "+gmp-%Y%d%m_%H%M")
TEST_ARGS=""
# Convert kind cluster name to required regex if necessary.
KIND_CLUSTER=`echo ${GO_TEST} | sed -r 's/([A-Z])/-\L\1/g' | sed 's/^-//'`
KUBE_CONTEXT=kind-${KIND_CLUSTER}
KUBECONFIG=/tmp/${KIND_CLUSTER}-config
KUBECTL="kubectl --context kind-${KIND_CLUSTER} --kubeconfig=${KUBECONFIG}"
# Ensure a unique label on any test data sent to GCM.
GMP_CLUSTER=$TAG_NAME
if [ -f "$GOOGLE_APPLICATION_CREDENTIALS" ]; then
  PROJECT_ID=$(jq -r '.project_id' ${GOOGLE_APPLICATION_CREDENTIALS})
else
  echo ">>> no credentials specified. running without GCM validation"
  TEST_ARGS="${TEST_ARGS} -skip-gcm"
  GOOGLE_APPLICATION_CREDENTIALS="/tmp/credentials-${KIND_CLUSTER}.json"
  # Create mock credentials.
  # Source: https://github.com/google/oauth2l/blob/v1.3.0/integration/fixtures/fake-service-account.json
  # This is necessary for any e2e tests that don't have access to GCP
  # credentials. We were getting away with this by running on networks
  # with access to the GCE metadata server IP to supply them:
  # https://github.com/googleapis/google-cloud-go/blob/56d81f123b5b4491aaf294042340c35ffcb224a7/compute/metadata/metadata.go#L39
  # However, running without this access (e.g. on Github Actions) causes
  # a failure from:
  # https://cs.opensource.google/go/x/oauth2/+/master:google/default.go;l=155;drc=9780585627b5122c8cc9c6a378ac9861507e7551
  cat > $GOOGLE_APPLICATION_CREDENTIALS <<EOL
{
  "type": "service_account",
  "private_key_id": "abc",
  "private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDY3E8o1NEFcjMM\nHW/5ZfFJw29/8NEqpViNjQIx95Xx5KDtJ+nWn9+OW0uqsSqKlKGhAdAo+Q6bjx2c\nuXVsXTu7XrZUY5Kltvj94DvUa1wjNXs606r/RxWTJ58bfdC+gLLxBfGnB6CwK0YQ\nxnfpjNbkUfVVzO0MQD7UP0Hl5ZcY0Puvxd/yHuONQn/rIAieTHH1pqgW+zrH/y3c\n59IGThC9PPtugI9ea8RSnVj3PWz1bX2UkCDpy9IRh9LzJLaYYX9RUd7++dULUlat\nAaXBh1U6emUDzhrIsgApjDVtimOPbmQWmX1S60mqQikRpVYZ8u+NDD+LNw+/Eovn\nxCj2Y3z1AgMBAAECggEAWDBzoqO1IvVXjBA2lqId10T6hXmN3j1ifyH+aAqK+FVl\nGjyWjDj0xWQcJ9ync7bQ6fSeTeNGzP0M6kzDU1+w6FgyZqwdmXWI2VmEizRjwk+/\n/uLQUcL7I55Dxn7KUoZs/rZPmQDxmGLoue60Gg6z3yLzVcKiDc7cnhzhdBgDc8vd\nQorNAlqGPRnm3EqKQ6VQp6fyQmCAxrr45kspRXNLddat3AMsuqImDkqGKBmF3Q1y\nxWGe81LphUiRqvqbyUlh6cdSZ8pLBpc9m0c3qWPKs9paqBIvgUPlvOZMqec6x4S6\nChbdkkTRLnbsRr0Yg/nDeEPlkhRBhasXpxpMUBgPywKBgQDs2axNkFjbU94uXvd5\nznUhDVxPFBuxyUHtsJNqW4p/ujLNimGet5E/YthCnQeC2P3Ym7c3fiz68amM6hiA\nOnW7HYPZ+jKFnefpAtjyOOs46AkftEg07T9XjwWNPt8+8l0DYawPoJgbM5iE0L2O\nx8TU1Vs4mXc+ql9F90GzI0x3VwKBgQDqZOOqWw3hTnNT07Ixqnmd3dugV9S7eW6o\nU9OoUgJB4rYTpG+yFqNqbRT8bkx37iKBMEReppqonOqGm4wtuRR6LSLlgcIU9Iwx\nyfH12UWqVmFSHsgZFqM/cK3wGev38h1WBIOx3/djKn7BdlKVh8kWyx6uC8bmV+E6\nOoK0vJD6kwKBgHAySOnROBZlqzkiKW8c+uU2VATtzJSydrWm0J4wUPJifNBa/hVW\ndcqmAzXC9xznt5AVa3wxHBOfyKaE+ig8CSsjNyNZ3vbmr0X04FoV1m91k2TeXNod\njMTobkPThaNm4eLJMN2SQJuaHGTGERWC0l3T18t+/zrDMDCPiSLX1NAvAoGBAN1T\nVLJYdjvIMxf1bm59VYcepbK7HLHFkRq6xMJMZbtG0ryraZjUzYvB4q4VjHk2UDiC\nlhx13tXWDZH7MJtABzjyg+AI7XWSEQs2cBXACos0M4Myc6lU+eL+iA+OuoUOhmrh\nqmT8YYGu76/IBWUSqWuvcpHPpwl7871i4Ga/I3qnAoGBANNkKAcMoeAbJQK7a/Rn\nwPEJB+dPgNDIaboAsh1nZhVhN5cvdvCWuEYgOGCPQLYQF0zmTLcM+sVxOYgfy8mV\nfbNgPgsP5xmu6dw2COBKdtozw0HrWSRjACd1N4yGu75+wPCcX/gQarcjRcXXZeEa\nNtBLSfcqPULqD+h7br9lEJio\n-----END PRIVATE KEY-----\n",
  "client_email": "123-abc@developer.gserviceaccount.com",
  "client_id": "123-abc.apps.googleusercontent.com",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "http://localhost:8080/token"
}
EOL
fi
TEST_ARGS="${TEST_ARGS} -project-id=${PROJECT_ID} -location=${GMP_LOCATION} -cluster=${GMP_CLUSTER} -kubectx=${KUBE_CONTEXT} --kubeconfig=${KUBECONFIG}"

patch_operator_flags() {
  $KUBECTL patch deployment gmp-operator --namespace gmp-system --type json \
    --patch '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--project-id='${PROJECT_ID}'"}]'
  $KUBECTL patch deployment gmp-operator --namespace gmp-system --type json \
    --patch '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--location='${GMP_LOCATION}'"}]'
  $KUBECTL patch deployment gmp-operator --namespace gmp-system --type json \
    --patch '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--cluster='${GMP_CLUSTER}'"}]'
}

add_gcm_secret() {
  $KUBECTL create secret generic gcm-editor --namespace gmp-public --from-file=key.json=${GOOGLE_APPLICATION_CREDENTIALS}
  $KUBECTL rollout status deploy/gmp-operator --namespace gmp-system
  # TODO(pintohutch): add a timeout here.
  until $KUBECTL patch operatorconfig --namespace gmp-public config --type merge --patch '{"collection":{"credentials":{"name":"gcm-editor","key":"key.json"}},"rules":{"queryProjectID":"'${PROJECT_ID}'","credentials":{"name":"gcm-editor","key":"key.json"}}}'; do
    # Retry to handle startup issues where the operator needs to update webhook ca bundles at runtime.
    echo ">>> OperatorConfig patch failed, retrying..."
    sleep 5
  done
}

ensure_registry() {
  echo ">>> ensuring docker registry"
  if [ "$(docker inspect -f '{{.State.Running}}' "${REGISTRY_NAME}" 2>/dev/null || true)" != 'true' ]; then
    docker run \
      -d --restart=always -p "127.0.0.1:${REGISTRY_PORT}:5000" --network bridge --name "${REGISTRY_NAME}" \
      registry:2
  fi
}

create_kind_cluster() {
  echo ">>> creating kind cluster"
  cat <<EOF | kind create cluster --kubeconfig ${KUBECONFIG} --name ${KIND_CLUSTER} --config=-
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
  done
}

# Set up local image registry and tag and push images to it.
# Finally update the install manifests to reference those images.
docker_tag_push $BINARIES
update_manifests $BINARIES

# Set up kind cluster and connect it to the local registry.
kind delete cluster --name ${KIND_CLUSTER} --kubeconfig ${KUBECONFIG}
create_kind_cluster
add_registry_to_nodes
connect_registry

# Install managed collection from install manifests.
echo ">>> deploying static resources"
$KUBECTL apply -f "${REPO_ROOT}/manifests/setup.yaml"
$KUBECTL apply -f "${REPO_ROOT}/manifests/operator.yaml"

# Add required flags: project_id, location, and cluster to operator deployment.
patch_operator_flags
# Create and specify secret for GCM access.
add_gcm_secret

# Install example app.
# TODO(pintohutch): do we need to deploy this every time?
$KUBECTL apply -f "${REPO_ROOT}/examples/instrumentation/go-synthetic/go-synthetic.yaml"
$KUBECTL rollout status deploy/go-synthetic

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
