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

SCRIPT_DIR=$(realpath $(dirname "${BASH_SOURCE[0]}"))
BUILD_DIR="$SCRIPT_DIR/build"
STATIC_DIR="$SCRIPT_DIR/static"

rm -rf $BUILD_DIR

# We need to preserve timestamps as otherwise node_modules/ may be falsely re-fetched
# which defeats the purpose of vendoring them.
# This is because make will check whether yarn.lock and package.json are older than
# node_modules/
cp -r --preserve=timestamps $SCRIPT_DIR/../../third_party/prometheus_ui/base $BUILD_DIR
cp -r $SCRIPT_DIR/../../third_party/prometheus_ui/override/* $BUILD_DIR/web/ui/react-app/

make -C "$BUILD_DIR" assets
rm -rf "$STATIC_DIR/react"
cp -r "$BUILD_DIR/web/ui/static/react" "$STATIC_DIR/react"

cd "$SCRIPT_DIR"
go run -mod=vendor generate_assets.go
