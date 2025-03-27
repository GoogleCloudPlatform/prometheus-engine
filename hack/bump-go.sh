#!/bin/bash
# Copyright 2025 Google LLC
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

# Bump Go images
GOLANG_REPO=google-go.pkg.dev/golang
GOLANG_TAG=$(go tool gcrane ls ${GOLANG_REPO} --json | jq --raw-output '.tags[]' | sort -V | tail -n1)
GOLANG_DIGEST=$(crane digest "${GOLANG_REPO}:${GOLANG_TAG}")
GOLANG_REF="${GOLANG_REPO}:${GOLANG_TAG}@${GOLANG_DIGEST}"
echo "${GOLANG_REF}"
find ./cmd ./examples ./hack -name Dockerfile -exec \
  sed -E "s#google-go\.pkg\.dev/golang:([0-9]+\.[0-9]+\.[0-9+][^@ ]*)?(@sha256:[0-9a-f]+)?#${GOLANG_REF}#g" -i {} \;

# Bump golangci-lint
go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
LINTER_TAG=$(go list -mod=readonly -m github.com/golangci/golangci-lint/v2 | awk '{print $2}')
echo "golangci-lint@${LINTER_TAG}"
go tool yq -i ".jobs[\"golangci-lint\"].steps[2].with.version = \"${LINTER_TAG}\"" .github/workflows/presubmit.yml
