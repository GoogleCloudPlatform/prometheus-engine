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
TEST_ARGS="-image-tag=${TAG_NAME} -registry-name=${REGISTRY_NAME} -registry-port=${REGISTRY_PORT}"
# Ensure a unique label on any test data sent to GCM.
GMP_CLUSTER=$TAG_NAME
if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  PROJECT_ID=$(jq -r '.project_id' "${GOOGLE_APPLICATION_CREDENTIALS}")
else
  echo ">>> no credentials specified. running without GCM validation"
  TEST_ARGS="${TEST_ARGS} -skip-gcm"
fi
TEST_ARGS="${TEST_ARGS} -project-id=${PROJECT_ID} -location=${GMP_LOCATION} -cluster=${GMP_CLUSTER}"
if [[ "${KIND_PERSIST:-0}" == "1" ]]; then
  TEST_ARGS="${TEST_ARGS} -kind-persist"
fi

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

# Update the install manifests to reference those images.
update_manifests $BINARIES

# Run the go tests.
echo ">>> executing gmp e2e tests: ${GO_TEST:-.}"
go test -v -timeout 10m "${REPO_ROOT}/e2e" -run "${GO_TEST:-.}" -args ${TEST_ARGS}
