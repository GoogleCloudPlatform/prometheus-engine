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

BASE_TAG=v2.26.0

SCRIPT_DIR=$(realpath $(dirname "${BASH_SOURCE[0]}"))
BUILD_DIR="$SCRIPT_DIR/build"
OVERRIDE_DIR="$SCRIPT_DIR/../../third_party/prometheus_ui/override"
REACT_APP_DIR="$BUILD_DIR/web/ui/react-app"

# Checkout the Prometheus UI at the given git tag into a working directory
# and override files provided in the diff/ directory.
mkdir -p $BUILD_DIR
cd $BUILD_DIR

if [[ $(git describe --tags) != "$BASE_TAG" ]]; then
  rm -rf ./*
  git clone \
    --branch "$BASE_TAG" \
    --depth 1 \
    https://github.com/prometheus/prometheus.git ./
fi

# Ensure overrides that are removed are properly reverted to the original state.
git clean -fd
git reset --hard HEAD

# Apply overrides.
cp -r "$OVERRIDE_DIR"/* "$REACT_APP_DIR"

# Then run the desired command against the final result.
cd $REACT_APP_DIR
"$@"