#!/usr/bin/env bash

# Copyright 2024 Google LLC
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

REPO_ROOT=$(realpath $(dirname "${BASH_SOURCE[0]}")/..)

# First argument is expected to be the name of our binary name after ./cmd/.
BIN_PATH=$1

# Print the command --help, but remove the full path (which is in tmp dir when
# build through go run).
go run "${REPO_ROOT}/cmd/${BIN_PATH}" --help 2>&1 >/dev/null | sed 's/^Usage of \/.*\//Usage of /'
