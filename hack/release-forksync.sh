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

TODO

NOTE: The script is idempotent; to force it to recreate local artifacts (e.g. local clones, remote branches it created), remove the artifact you want to recreate.

Example use:
*

Variables:
* CHECKOUT_DIR (required) - Local working directory e.g. for local clones.
* BASE_BRANCH (required) - Fork branch considered as a base for the forked changes.
* UPSTREAM_TAG (required) - Upstream tag to synchronize (rebase) to; this also controls the name of the eventual branch to use for the sync (release-$UPSTREAM_TAG-gmp).
* PR_BRANCH (default: $USER/cut-release-$UPSTREAM_TAG-gmp) - Upstream branch to push to.
_EOM
}

if (( $# > 0 )); then
  case $1 in
  help)
      usage
      ;;
  esac
fi

if [[ -z "${CHECKOUT_DIR}" ]]; then
  echo "❌  CHECKOUT_DIR environment variable is not set." >&2
  usage
  return 1
fi
if [[ -z "${BASE_BRANCH}" ]]; then
  echo "❌  BASE_BRANCH environment variable is not set." >&2
  usage
  return 1
fi
if [[ -z "${UPSTREAM_TAG}" ]]; then
  echo "❌  UPSTREAM_TAG environment variable is not set." >&2
  usage
  return 1
fi


REMOTE_URL=$(release-lib::remote_url_from_branch "${BASE_BRANCH}")
PROJECT=$(tmp=${REMOTE_URL##*/}; echo ${tmp%.git})
PR_BRANCH=${PR_BRANCH:-"${USER}/${BASE_BRANCH}-vulnfix"}

echo "🔄 Assuming ${PROJECT} with remote ${REMOTE_URL}; changes will be pushed to ${PR_BRANCH}"

DIR="${CHECKOUT_DIR}/${PROJECT}"
release-lib::idemp::clone "${DIR}"
