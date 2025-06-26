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

set -o errexit
set -o pipefail
set -o nounset

if [[ -n "${DEBUG_MODE:-}" ]]; then
	set -o xtrace
fi

SCRIPT_DIR="$(
	cd -- "$(dirname "$0")" >/dev/null 2>&1
	pwd -P
)"

usage() {
	local me
	me=$(basename "${BASH_SOURCE[0]}")
	cat <<_EOM
usage: ${me} <Dockerfile>

Update google-go.pkg.dev/golang base images in the given Dockerfile to the latest.

Example use:
* ${me} ./Dockerfile

Variables:
* LATEST_MINOR (optional) - only update to the given latest minor version.
* INCLUDE_RC (optional) - if set update update to the latest RC
_EOM
}

if (($# > 0)); then
	case $1 in
	help)
		usage
		;;
	esac
fi

if (($# != 1)); then
	echo "❌  Expected exactly one argument"
	usage
	exit 1
fi

DOCKERFILE="${1}"
echo "🔄  Detecting google-go.pkg.dev/golang image references..."
if grep -E "google-go\.pkg\.dev/golang" "${DOCKERFILE}"; then
	golang_tags=$(gcrane ls "google-go.pkg.dev/golang" --json | jq --raw-output '.tags[]' | sort -V)
	if [[ -z "${INCLUDE_RC:-}" ]]; then
		golang_tags=$(echo "${golang_tags}" | grep -v "rc.*")
	fi
	if [[ -n "${LATEST_MINOR:-}" ]]; then
		golang_tags=$(echo "${golang_tags}" | grep "${LATEST_MINOR}.*")
	fi
	latest_golang_tag=$(echo "${golang_tags}" | tail -n1)
	latest_golang_digest=$(crane digest "google-go.pkg.dev/golang:${latest_golang_tag}")
	latest_golang_image="google-go.pkg.dev/golang:${latest_golang_tag}@${latest_golang_digest}"
	echo "🔄  Ensuring ${latest_golang_image}..."
	if gsed -i -E "s#google-go\.pkg\.dev/golang:([0-9]+\.[0-9]+\.[0-9+][^@ ]*)?(@sha256:[0-9a-f]+)?#${latest_golang_image}#g" "${DOCKERFILE}"; then
		echo "✅  Done!"
		exit 0
	else
		echo "❌  sed didn't replace?"
		exit 1
	fi
fi
echo "❌  Nothing to do, no google-go.pkg.dev/golang image referenced in the ${DOCKERFILE}"
