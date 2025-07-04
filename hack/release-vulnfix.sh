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

SCRIPT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
source "${SCRIPT_DIR}/release-lib.sh"

usage() {
    local me
    me=$(basename "${BASH_SOURCE[0]}")
    cat <<_EOM
usage: ${me}

Attempt a minimal dependency upgrade to solve fixable vulnerabilities.

NOTE: The script is idempotent; to force it to recreate local artifacts (e.g. local clones, remote branches it created), remove the artifact you want to recreate.

Example use:
 * BRANCH=release/0.15 CHECKOUT_DIR=~/Repos/tmp-release bash hack/release-vulnfix.sh
 * BRANCH=release-2.45.3-gmp CHECKOUT_DIR=~/Repos/tmp-release bash hack/release-vulnfix.sh
 * BRANCH=release-0.27.0-gmp CHECKOUT_DIR=~/Repos/tmp-release bash hack/release-vulnfix.sh

Variables:
* BRANCH (required) - Release branch to work on; Project is auto-detected from this.
* CHECKOUT_DIR (required) - Local working directory e.g. for local clones.
* PR_BRANCH (default: $USER/$BRANCH-vulnfix) - Upstream branch to push to.
_EOM
}

if (( $# > 0 )); then
  case $1 in
  help)
      usage
      ;;
  esac
fi

# Check if the BRANCH environment variable is set.
if [[ -z "${BRANCH}" ]]; then
  echo "❌  BRANCH environment variable is not set." >&2
  usage
  return 1
fi

REMOTE_URL=$(release-lib::remote_url_from_branch "${BRANCH}")
PROJECT=$(tmp=${REMOTE_URL##*/}; echo ${tmp%.git})
PR_BRANCH=${PR_BRANCH:-"${USER}/${BRANCH}-vulnfix"}

echo "🔄 Assuming ${PROJECT} with remote ${REMOTE_URL}; changes will be pushed to ${PR_BRANCH}"

# Check if the BRANCH environment variable is set.
if [[ -z "${CHECKOUT_DIR}" ]]; then
  echo "❌  CHECKOUT_DIR environment variable is not set." >&2
  usage
  return 1
fi

DIR="${CHECKOUT_DIR}/${PROJECT}"
if [[ ! -d "${DIR}" ]]; then
  git clone -b "${BRANCH}" --single-branch "${REMOTE_URL}" "${DIR}"
  pushd "${DIR}"
  git checkout -b "${PR_BRANCH}"
  popd
fi

pushd "${DIR}"
if [[ "$(git symbolic-ref --short HEAD)" != "${PR_BRANCH}" ]]; then
    echo "❌  Malformed ${DIR}; expected ${PR_BRANCH}; remove of gix manually the ${DIR} and rerun." >&2
    exit 1
fi

# TODO: Make every command idempotent inside each function?
if [[ ! -f "${DIR}/vulnlist.txt" || -z $(cat "${DIR}/vulnlist.txt") ]]; then
  release-lib::vulnlist "${DIR}"
fi

if [[ "no vulnerabilities" != $(cat "${DIR}/vulnlist.txt") ]]; then
  # Attempt to update + go mod tidy.
  release-lib::gomod_vulnfix "${DIR}"
  git add go.mod go.sum

  # Check if that helped.
  echo "⚠️ This will fail on older branches with vendoring; in this case, simply go to ${DIR}, run 'go mod vendor' and rerun."
  release-lib::vulnlist "${DIR}"
  if [[ "no vulnerabilities" != $(cat "${DIR}/vulnlist.txt") ]]; then
     echo "❌  After go mod update some vulnerabilities are still found; go to ${DIR} and resolve it manually and remove the ./vulnlist.txt file and rerun." >&2
     exit 1
  fi
fi

if ! git diff-index --quiet --cached HEAD; then
  release-lib::git_commit_amend_match "fix: fix ${BRANCH} vulnerabilities"
fi

if [[ "$(git fetch && git rev-parse HEAD)" != "$(git rev-parse @{u})" ]]; then
  # TODO: Force with ack (required by amends/recreations)
  # TODO: Potentially use ghclient for PR ops
  git push origin "${PR_BRANCH}"

else
  echo "⚠️ Nothing to do; no vulnerabilities and nothing to commit"
  exit 1
fi
