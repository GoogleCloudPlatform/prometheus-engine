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

# TODO(bwplotka): Clean err on missing deps e.g. gsed.

SCRIPT_DIR="$(
	cd -- "$(dirname "$0")" >/dev/null 2>&1
	pwd -P
)"
source "${SCRIPT_DIR}/lib.sh"

usage() {
	local me
	me="${BASH_SOURCE[0]}"
	cat <<_EOM
usage: ${me}

Release the RC.

NOTE: The script is idempotent; to force it to recreate local artifacts (e.g. local clones, remote branches it created), remove the artifact you want to recreate.

Example use:
 * BRANCH=release/0.15 TAG=v0.15.4-rc.0 CHECKOUT_DIR=~/Repos/tmp-release ${me}
 * BRANCH=release-2.45.3-gmp TAG=v2.45.3-gmp.13-rc.0 CHECKOUT_DIR=~/Repos/tmp-release ${me}
 * BRANCH=release-0.27.0-gmp TAG=v0.27.0-gmp.4-rc.0 CHECKOUT_DIR=~/Repos/tmp-release ${me}

Variables:
* BRANCH (required) - Release branch to work on; Project is auto-detected from this.
* CHECKOUT_DIR or DIR (required) - Local working directory e.g. for local clones. DIR is a working dir, CHECKOUT_DIR sets DIR to CHECKOUT_DIR/<project name> from remote URL.
* TAG (optional) - Tag to release. If empty next tag version will be detected (double check this!)
* FORCE_NEW_PATCH_VERSION (optional) - If not empty, forces a new patch version as a new TAG (if TAG is empty).
_EOM
}

if (($# > 0)); then
	case $1 in
	help)
		usage
		exit 0
		;;
	esac
fi

# Check if the BRANCH environment variable is set.
if [[ -z "${BRANCH}" ]]; then
	echo "❌  BRANCH environment variable is not set."
	usage
	exit 1
fi

REMOTE_URL=$(release-lib::remote_url_from_branch "${BRANCH}")
PROJECT=$(
	tmp=${REMOTE_URL##*/}
	echo ${tmp%.git}
)
PR_BRANCH=${BRANCH} # Same as branch because we push directly, without PR as per our process.

echo "🔄 Assuming ${PROJECT} with remote ${REMOTE_URL}; changes will be pushed directly to ${PR_BRANCH}"

if [[ -z "${CHECKOUT_DIR:-}" && -z "${DIR:-}" ]]; then
	echo "❌  CHECKOUT_DIR or DIR environment variable has to be set."
	usage
	exit 1
fi
DIR=${DIR:-"${CHECKOUT_DIR}/${PROJECT}"}

release-lib::idemp::clone "${DIR}" "${BRANCH}" "${PR_BRANCH}"

pushd "${DIR}"

if [[ -z "${TAG:-}" ]]; then
	TAG=$(release-lib::next_release_tag "${DIR}")
	echo "✅  Detected next release tag: ${TAG}"
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

if ! release-lib::confirm "About to create a commit and a local git tag for ${TAG} in ${DIR} on ${PR_BRANCH}; should I continue?"; then
	exit 1
fi

# Commit if anything is staged.
release-lib::idemp::git_commit_amend_match "chore: prepare for ${TAG} release"

# Check if tag exists.
if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
	# Tag exists, but is it tagged for the current HEAD?
	if [[ "$(git rev-parse HEAD)" != "$(git rev-list -n 1 "${TAG}")" ]]; then
		echo "❌  Tag ${TAG} exists already locally, not pointing to the HEAD; consider 'git tag -d' to remove it and rerun."
		exit 1
	fi
else
	echo "🔄  Creating a signed tag ${TAG}..."
	# explicit TTY is often needed on Macs.
	# TODO(bwplotka): Consider adding v0.x second tag for Prometheus fork (similar to how v0.300 Prometheus releases are structured).
	# This is to have a little bit cleaner prometheus-engine go.mod version against the fork.
	GPG_TTY=$(tty) git tag -s "${TAG}" -m "${TAG}"
fi

if release-lib::needs_push "${PR_BRANCH}" "${BRANCH}" || ! git ls-remote --tags --exit-code origin "refs/tags/${TAG}" >/dev/null; then
	if release-lib::confirm "About to git push state from ${DIR} to origin/${PR_BRANCH}; then ${TAG}; are you sure?"; then
		git push origin "${PR_BRANCH}"
		git push origin "${TAG}"
	fi
else
	exit 1
fi
