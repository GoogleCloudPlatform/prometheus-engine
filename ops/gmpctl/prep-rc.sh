#!/usr/bin/env bash
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

# NOTE for contributors: Bash is funky, but sometimes more readable than Go/easier to iterate.
# Eventually, we could rewrite more critical pieces to Go, but you're welcome to add some quick
# pieces in bash to automate some stuff.

set -o errexit
set -o pipefail
set -o nounset

if [[ -n "${DEBUG_MODE:-}" ]]; then
	set -o xtrace
fi

# TODO(bwplotka): Finding correct script dir is not so trivial.
if [[ -z "${SCRIPT_DIR}" ]]; then
	log_err "SCRIPT_DIR envvar is required."
	exit 1
fi

source "${SCRIPT_DIR}/lib.sh"

if [[ -z "${DIR}" ]]; then
	log_err "DIR envvar is required."
	exit 1
fi

if [[ -z "${BRANCH}" ]]; then
	log_err "BRANCH envvar is required."
	exit 1
fi

if [[ -z "${TAG}" ]]; then
	log_err "TAG envvar is required."
	exit 1
fi

if [[ -z "${PROJECT}" ]]; then
	log_err "PROJECT envvar is required."
	exit 1
fi

if [[ "${PROJECT}" == "prometheus-engine" ]]; then
	CLEAN_TAG="${TAG%-rc.*}"
	CLEAN_TAG="${CLEAN_TAG#v}"
	if [[ "${BRANCH}" == "release/0.12" ]]; then
		# A bit different flow.
		chart_file="${DIR}/charts/operator/Chart.yaml"
		echo "🔄  Ensuring ${CLEAN_TAG} on ${chart_file}..."
		if ! gsed -i -E "s#appVersion:.*#appVersion: ${CLEAN_TAG}#g" "${chart_file}"; then
			# TODO: This is flaky, no failing actually on no match. Common bug is
			echo "❌  sed didn't replace?"
			exit 1
		fi

		chart_file="${DIR}/charts/rule-evaluator/Chart.yaml"
		echo "🔄  Ensuring ${CLEAN_TAG} on ${chart_file}..."
		if ! gsed -i -E "s#appVersion:.*#appVersion: ${CLEAN_TAG}#g" "${chart_file}"; then
			# TODO: This is flaky, no failing actually on no match. Common bug is
			echo "❌  sed didn't replace?"
			exit 1
		fi
	else
		# 0.12+
		values_file="${DIR}/charts/values.global.yaml"
		echo "🔄  Ensuring ${CLEAN_TAG} on ${values_file}..."
		if ! gsed -i -E "s#version:.*#version: ${CLEAN_TAG}#g" "${values_file}"; then
			# TODO: This is flaky, no failing actually on no match. Common bug is
			echo "❌  sed didn't replace?"
			exit 1
		fi
	fi
	# For versions with export embedded.
	if [[ -f "${DIR}/pkg/export/export.go" ]]; then
		echo "🔄  Ensuring ${TAG} in ${DIR}/pkg/export/export.go mainModuleVersion..."
		if ! gsed -i -E "s#mainModuleVersion = .*#mainModuleVersion = \"${TAG}\"#g" "${DIR}/pkg/export/export.go"; then
			# TODO: This is flaky, no failing actually on no match. Common bug is
			echo "❌  sed didn't replace?"
			exit 1
		fi
	fi

	release-lib::manifests_regen "${DIR}"
	git add --all
else
	# Prometheus and Alertmanager fork needs just a correct version in the VERSION file,
	# so the binary build (go_build_info) metrics and flags are correct.
	temp=${TAG#v} # Remove v and then -rc.* suffix.
	echo "${temp%-rc.*}" >VERSION
	git add VERSION
fi
