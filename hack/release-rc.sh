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
* TAG (required) - Tag to release.
* CHECKOUT_DIR (required) - Local working directory e.g. for local clones.
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
	echo "❌  BRANCH environment variable is not set." >&2
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

# Check if the BRANCH environment variable is set.
if [[ -z "${CHECKOUT_DIR}" ]]; then
	echo "❌  CHECKOUT_DIR environment variable is not set." >&2
	usage
	exit 1
fi

# TODO(bwplotka): Auto-detect this.
if [[ -z "${TAG}" ]]; then
	echo "❌  TAG environment variable is not set." >&2
	usage
	exit 1
fi

DIR="${CHECKOUT_DIR}/${PROJECT}"
release-lib::idemp::clone "${DIR}" "${BRANCH}" "${PR_BRANCH}"

pushd "${DIR}"

if [[ "${PROJECT}" == "prometheus-engine" ]]; then
	pushd "${SCRIPT_DIR}"
	go run "./prepare_rc" -dir "${DIR}" -tag "${TAG}"
	popd
	"${DIR}/hack/presubmit.sh" manifests
	git add --all
else
	# Prometheus and Alertmanager fork needs just a correct version in the VERSION file,
	# so the binary build (go_build_info) metrics and flags are correct.
	temp=${TAG#v} # Remove v and then -rc.* suffix.
	echo "${temp%-rc.*}" >VERSION
	git add VERSION
fi

# Commit if anything is staged.
release-lib::idemp::git_commit_amend_match "chore: prepare for ${TAG} release"

# Check if tag exists.
if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
	# Tag exists, but is it tagged for the current HEAD?
	if [[ "$(git rev-parse HEAD)" != "$(git rev-list -n 1 "${TAG}")" ]]; then
		echo "❌  Tag ${TAG} exists already locally, not pointing to the HEAD; consider 'git tag -d' to remove it and rerun." >&2
		exit 1
	fi
else
	echo "🔄 Creating a signed tag ${TAG}..."
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
