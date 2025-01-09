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

set -o errexit
set -o nounset
set -o pipefail

source .bingo/variables.env

VALUES=charts/values.global.yaml
VERSION=$(${YQ} '.version' "$VALUES")

check_image() {
  IMAGE=$1
  TAG=$2
  GMP_VERSIONED=${3:-false}

  LATEST=$(docker run gcr.io/go-containerregistry/crane ls "${IMAGE}" | grep "^v[0-9]" | sort -V | tail -1)

  if [[ $GMP_VERSIONED = true && ! "$TAG" =~ ^v${VERSION}.*$ ]]; then
    printf "GMP Version is %q, but tag %q of %q does not match\n" "$VERSION" "$TAG" "$IMAGE" && exit 1
  fi

  if [ "$TAG" != "$LATEST" ]; then
    printf "%s is %q, latest is %q" "$IMAGE" "$TAG" "$LATEST"
  fi
  docker manifest inspect "${IMAGE}:${TAG}" > /dev/null

  printf "%s:%s verified\n" "$IMAGE" "$TAG"
}

ALERTMANAGER_IMAGE=$(${YQ} '.images.alertmanager.image' "$VALUES")
ALERTMANAGER_TAG=$(${YQ} '.images.alertmanager.tag' "$VALUES")
check_image "$ALERTMANAGER_IMAGE" "$ALERTMANAGER_TAG"

CONFIG_RELOADER_IMAGE=$(${YQ} '.images.configReloader.image' "$VALUES")
CONFIG_RELOADER_TAG=$(${YQ} '.images.configReloader.tag' "$VALUES")
check_image "$CONFIG_RELOADER_IMAGE" "$CONFIG_RELOADER_TAG" true

DATASOURCE_SYNCER_IMAGE=$(${YQ} '.images.datasourceSyncer.image' "$VALUES")
DATASOURCE_SYNCER_TAG=$(${YQ} '.images.datasourceSyncer.tag' "$VALUES")
check_image "$DATASOURCE_SYNCER_IMAGE" "$DATASOURCE_SYNCER_TAG" true

OPERATOR_IMAGE=$(${YQ} '.images.operator.image' "$VALUES")
OPERATOR_TAG=$(${YQ} '.images.operator.tag' "$VALUES")
check_image "$OPERATOR_IMAGE" "$OPERATOR_TAG" true

PROMETHEUS_IMAGE=$(${YQ} '.images.prometheus.image' "$VALUES")
PROMETHEUS_TAG=$(${YQ} '.images.prometheus.tag' "$VALUES")
check_image "$PROMETHEUS_IMAGE" "$PROMETHEUS_TAG"

RULE_EVALUATOR_IMAGE=$(${YQ} '.images.ruleEvaluator.image' "$VALUES")
RULE_EVALUATOR_TAG=$(${YQ} '.images.ruleEvaluator.tag' "$VALUES")
check_image "$RULE_EVALUATOR_IMAGE" "$RULE_EVALUATOR_TAG" true

echo "All images verified"
