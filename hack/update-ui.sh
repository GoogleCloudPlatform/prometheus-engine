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

# This script updates the code of the Prometheus UI in third_party/prometheus_ui/base
# to the state of the provided upstream release tag. Should be manually run outside of
# build automation.
#
# Example: ./update-ui.sh v2.26.1

set -o errexit
set -o nounset
set -o pipefail

BASE_TAG=$1

SCRIPT_DIR=$(realpath $(dirname "${BASH_SOURCE[0]}"))
BASE_DIR="$SCRIPT_DIR/../third_party/prometheus_ui/base"

rm -rf $BASE_DIR
mkdir $BASE_DIR
cd $BASE_DIR

git clone \
  --branch "$BASE_TAG" \
  --depth 1 \
  https://github.com/prometheus/prometheus.git ./

# Enable better pattern matching to only keep files we care about.
shopt -s dotglob
shopt -s extglob

rm -rf \
  !(web|Makefile*|scripts|VERSION) \
  web/!(ui) web/ui/!(react-app|static) web/ui/static/!(react) web/ui/react-app/.gitignore \
  scripts/!(build_react_app.sh)

# Download modules according to lock file.
cd web/ui/react-app && yarn install --frozen-lockfile