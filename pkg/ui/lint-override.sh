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

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/../..
SCRIPT_DIR=$(realpath $(dirname "${BASH_SOURCE[0]}"))

# eslint depends not only on eslintrc.json configuration, but also various versions of plugins and components.
# For this reason we have to symlink, then lint in exactly same directory, then rm symlink to have
# override files linted as Prometheus UI files (checked during build).
BASE_REACT_DIR=$SCRIPT_DIR/../../third_party/prometheus_ui/base/web/ui/react-app
OVERRIDE_DIR=$SCRIPT_DIR/../../third_party/prometheus_ui/override

ln -s $OVERRIDE_DIR $BASE_REACT_DIR/src/

cd $BASE_REACT_DIR
npx eslint --fix ./src/override/**/*.tsx

rm $BASE_REACT_DIR/src/override

