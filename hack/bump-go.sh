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

REPO=google-go.pkg.dev/golang
TAG=$(crane ls ${REPO} | tail -n1)
DIGEST=$(crane digest "${REPO}:${TAG}")
IMAGE="${REPO}:${TAG}@${DIGEST}"
echo "$IMAGE"
find ./cmd ./examples ./hack -name Dockerfile -exec \
  sed -E "s#google-go\.pkg\.dev/golang:[0-9]+\.[0-9]+\.[0-9+][^@ ]*(@sha256:[0-9a-f]+)?#${IMAGE}#g" -i {} \;
