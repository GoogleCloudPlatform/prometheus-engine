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

# Useful functions for release scripts.

set -o errexit
set -o pipefail
set -o nounset

if [[ -n "${DEBUG_MODE:-}" ]]; then
  set -o xtrace
fi

SCRIPT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

release-lib::remote_url_from_branch() {
  local branch=$1
  # Check if the BRANCH environment variable is set.
  if [[ -z "${branch}" ]]; then
    echo "❌  branch is required." >&2
    return 1
  fi

  if [[ "${branch}" =~ ^release-(2|3)\.[0-9]+\.[0-9]+-gmp$ ]]; then
    echo "git@github.com:GoogleCloudPlatform/prometheus.git"
  elif [[ "${branch}" =~ ^release-0\.[0-9]+\.[0-9]+-gmp$ ]]; then
    echo "git@github.com:GoogleCloudPlatform/alertmanager.git"
  elif [[ "${branch}" =~ ^release/0\.[0-9]+$ ]]; then
    echo "git@github.com:GoogleCloudPlatform/prometheus-engine.git"
  else
    echo "❌  No matching remote URL found for branch='$BRANCH'" >&2
    return 1
  fi
}

release-lib::vulnlist() {
  local dir=$1
  if [[ -z "${dir}" ]]; then
    echo "❌  dir is required." >&2
    return 1
  fi

  echo "🔄 Detecting Go vulnerabilities to fix..."
  # TODO(bwplotka): Capture correct Go version.
  # TODO(bwplotka): api.text is useful, document how to obtain it.
  pushd "${SCRIPT_DIR}/vulnupdatelist/"
    go run "./..." \
      -go-version=1.24.2 \
      -only-fixed \
      -dir="${dir}" \
      -nvd-api-key="$(cat "./api.text")" | tee "${dir}/vulnlist.txt"
   if [[ -z $(cat "${dir}/vulnlist.txt") ]]; then
      # Print this, otherwise error on the above might keep this file mistakenly empty.
      echo "no vulnerabilities" > "${dir}/vulnlist.txt"
    fi
  popd
}

release-lib::gomod_vulnfix() {
  local dir=$1
  if [[ -z "${dir}" ]]; then
    echo "❌  dir is required." >&2
    return 1
  fi

  local vuln_file="${dir}/vulnlist.txt"
  if [[ ! -f "${vuln_file}" ]]; then
    echo "❌  no ${vuln_file} file found" >&2
    return 1
  fi

  if [[ "no vulnerabilities" == $(cat "${vuln_file}") ]]; then
     echo "❌  ${vuln_file} shows no vulnerabilities" >&2
     return 1
  fi

  # Read the vulnerability file line by line.
  # The `|| [[ -n "$line" ]]` part handles the case where the last line doesn't have a newline.
  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip any empty lines in the input file.
    if [ -z "$line" ]; then
      continue
    fi

   mod=$(echo "$line" | awk '{print $2}')
   mod_path=$(echo "${mod}" | cut -d'@' -f1)
   desired_version=$(echo "${mod}" | cut -d'@' -f2)

    if [[ -z "${mod_path}" ]] || [[ -z "${desired_version}" ]]; then
      echo "⚠️ Skipping malformed line: $line"
      continue
    fi

    echo "🔄 Updating module '${mod_path}' to version '${desired_version}'..."
    gsed -i.bak "s|\(	${mod_path} \).*|\1${desired_version}|" "${dir}/go.mod"
  done < "${vuln_file}"
  echo "🔄 Resolving ${dir}/go.mod..."
  pushd "${dir}"
    go mod tidy
  popd
  rm "${dir}/go.mod.bak"
}

release-lib::git_commit_amend_match() {
  local message=$1
  if [[ -z "${message}" ]]; then
    echo "❌  message is required." >&2
    return 1
  fi
  if [[ "$(git log -1 --pretty=%s)" == "${message}" ]]; then
    git commit -s --amend -m "${message}"
  else
    git commit -sm "${message}"
  fi
}
