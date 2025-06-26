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
# TODO(bwplotka): Consider automation for npm and docker images (Go, debian, similar to bump-go.sh)

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

Attempt a minimal dependency upgrade to solve fixable vulnerabilities.

* Docker images:
  * Distros use latest tag so rebuilding takes latest, nothing to do.
  * google-go.pkg.dev/golang images are updated to the latest minor version using docker-bump-images.sh
* Manifests
  * distroless bumped to latest (although our component tooling is capable of bumpting this too)
* Go deps: Upgrade to minimal required version per a known fixable vulnerability.
* Npm deps: Not implemented.

NOTE: The script is idempotent; to force it to recreate local artifacts (e.g. local clones, remote branches it created), remove the artifact you want to recreate.

Example use:
 * BRANCH=release/0.15 CHECKOUT_DIR=~/Repos/tmp-release ${me}
 * BRANCH=release-2.45.3-gmp CHECKOUT_DIR=~/Repos/tmp-release ${me}
 * BRANCH=release-0.27.0-gmp CHECKOUT_DIR=~/Repos/tmp-release ${me}

Variables:
* BRANCH (required) - Release branch to work on; Project is auto-detected from this.
* CHECKOUT_DIR or DIR (required) - Local working directory e.g. for local clones. DIR is a working dir, CHECKOUT_DIR sets DIR to CHECKOUT_DIR/<project name> from remote URL.
* PR_BRANCH (default: USER/BRANCH-vulnfix) - Upstream branch to push to (user-confirmed first).
* SYNC_DOCKERFILES_FROM - optional branch name to sync manifests for each dockerfile.
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
PR_BRANCH=${PR_BRANCH:-"${USER}/${BRANCH}-vulnfix"}

echo "🔄  Assuming ${PROJECT} with remote ${REMOTE_URL}; changes will be pushed to ${PR_BRANCH}"

if [[ -z "${CHECKOUT_DIR:-}" && -z "${DIR:-}" ]]; then
	echo "❌  CHECKOUT_DIR or DIR environment variable has to be set."
	usage
	exit 1
fi
DIR=${DIR:-"${CHECKOUT_DIR}/${PROJECT}"}

release-lib::idemp::clone "${DIR}" "${BRANCH}" "${PR_BRANCH}"

readarray -t DOCKERFILES < <(release-lib::dockerfiles "${DIR}")

# Sync dockerfiles if needed.
if [[ -n "${SYNC_DOCKERFILES_FROM:-}" ]]; then
	pushd "${DIR}"
	for dockerfile in "${DOCKERFILES[@]}"; do
		# TODO: Should we ensure SYNC_DOCKERFILES_FROM if it's a branch is up to data with origin?
		echo "🔄  Syncing ${dockerfile} from ${SYNC_DOCKERFILES_FROM}"
		git checkout "${SYNC_DOCKERFILES_FROM}" -- "${dockerfile}"
	done
	popd
fi

# Docker images bumps.

# Get first dockerfile Go version. We will use this version to find minor version to stick to.
go_version=$(release-lib::dockerfile_go_version "${DOCKERFILES[0]}")
if [[ -z "${go_version}" ]]; then
	echo "❌  can't find any golang image in ${DOCKERFILES[0]}"
	exit 1
fi

# TODO: git add charts & vendor for old projects.

# Update our images.
for dockerfile in "${DOCKERFILES[@]}"; do
	release-lib::dockerfile_update_image "${dockerfile}" "google-go.pkg.dev/golang" $(echo "${go_version}" | cut -d '.' -f 1-2)
	release-lib::dockerfile_update_image "${dockerfile}" "gke.gcr.io/gke-distroless/libc" "gke_distroless_"
	pushd "${DIR}"
	  git add "${dockerfile}"
	popd
done

# bash manifest bump.
# Exclude 0.12 as values were inlined with each part, easy to manually sed for old versions.
if [[ "${PROJECT}" == "prometheus-engine" && "${BRANCH}" != "release/0.12" ]]; then
	release-lib::idemp::manifests_bash_image_bump "${DIR}"
fi

# Go vulnerabilities.
vuln_file="${DIR}/.git/vulnlist.txt"
pushd "${DIR}"

release-lib::idemp::vulnlist "${DIR}" "${vuln_file}"

if [[ "no vulnerabilities" != $(cat "${vuln_file}") ]]; then
	# Attempt to update + go mod tidy.
	release-lib::gomod_vulnfix "${DIR}" "${vuln_file}"
	git add go.mod go.sum

	# Check if that helped.
	echo "⚠️  This will fail on older branches with vendoring; in this case, simply go to ${DIR}, run 'go mod vendor' and rerun."
	release-lib::vulnlist "${DIR}" "${vuln_file}"
	if [[ "no vulnerabilities" != $(cat "${vuln_file}") ]]; then
		echo "❌  After go mod update some vulnerabilities are still found; go to ${DIR} and resolve it manually and remove the ./vulnlist.txt file and rerun."
		exit 1
	fi
fi

# TODO: Warn of unstaged files at this point.

# Commit if anything is staged.
msg="google patch[deps]: fix ${BRANCH} vulnerabilities"
if [[ "${PROJECT}" == "prometheus-engine" ]]; then
	msg="fix: fix ${BRANCH} vulnerabilities"
fi
release-lib::idemp::git_commit_amend_match "${msg}"

if release-lib::needs_push "${PR_BRANCH}" "${BRANCH}"; then
	if release-lib::confirm "About to FORCE git push from ${DIR} to origin/${PR_BRANCH}; are you sure?"; then
		git push --force origin "${PR_BRANCH}"
	fi
else
	exit 1
fi
