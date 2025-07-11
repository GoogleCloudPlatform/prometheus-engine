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

release-lib::confirm() {
	local prompt_message="${1:-Are you sure?}"

	# -p: Display the prompt string.
	# -r: Prevents backslash interpretation.
	# -n 1: Read only one character.
	read -p "$prompt_message [y/n]: " -r -n 1 response
	echo # Ensures the cursor moves to the next line after input.
	case "$response" in
	[yY])
		return 0
		;;
	[nN])
		echo "❌  The action has been cancelled as requested."
		return 1
		;;
	*)
		echo "Invalid input. Exiting script." >&2
		exit 1
		;;
	esac
}

release-lib::idemp::clone() {
	local clone_dir=$1
	if [[ -z "${clone_dir}" ]]; then
		echo "❌  clone_dir variable is not set." >&2
		return 1
	fi
	if [[ -z "${PR_BRANCH}" ]]; then
		echo "❌  PR_BRANCH environment variable is not set." >&2
		return 1
	fi
	if [[ ! -d "${clone_dir}" ]]; then
		release-lib::clone "${clone_dir}"
	fi

	pushd "${clone_dir}"
	if [[ "$(git symbolic-ref --short HEAD)" != "${PR_BRANCH}" ]]; then
		echo "❌  Malformed ${DIR}; expected ${PR_BRANCH} got $(git symbolic-ref --short HEAD); remove or fix manually the ${DIR} and rerun." >&2
		return 1
	fi
	popd
}

release-lib::clone() {
	local clone_dir=$1
	if [[ -z "${clone_dir}" ]]; then
		echo "❌  clone_dir variable is not set." >&2
		return 1
	fi
	if [[ -z "${BRANCH}" ]]; then
		echo "❌  BRANCH environment variable is not set." >&2
		return 1
	fi
	if [[ -z "${REMOTE_URL}" ]]; then
		echo "❌  REMOTE_URL environment variable is not set." >&2
		return 1
	fi
	if [[ -z "${PR_BRANCH}" ]]; then
		echo "❌  PR_BRANCH environment variable is not set." >&2
		return 1
	fi
	# NOTE: We could add --single-branch but it would be a bit harder to use interactively.
	git clone -b "${BRANCH}" "${REMOTE_URL}" "${clone_dir}"
	if [[ "${BRANCH}" != "${PR_BRANCH}" ]]; then
		pushd "${clone_dir}"
		git checkout -b "${PR_BRANCH}"
		popd
	fi
}

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
		echo "❌  No matching remote URL found for branch=${branch}" >&2
		return 1
	fi
}

release-lib::upstream_remote_url() {
	local project=$1
	if [[ -z "${project}" ]]; then
		echo "❌  project is required." >&2
		return 1
	fi

	if [[ "${project}" == "prometheus" ]]; then
		echo "git@github.com:prometheus/prometheus.git"
	elif [[ "${project}" == "alertmanager" ]]; then
		echo "git@github.com:prometheus/alertmanager.git"
	else
		echo "❌  No matching remote URL found for project='${project}'" >&2
		return 1
	fi
}

release-lib::idemp::vulnlist() {
	local dir=$1
	if [[ -z "${dir}" ]]; then
		echo "❌  dir is required." >&2
		return 1
	fi

	if [[ ! -f "${dir}/vulnlist.txt" || -z $(cat "${dir}/vulnlist.txt") ]]; then
		release-lib::vulnlist "${dir}"
	else
		echo "⚠️ Using existing ${dir}/vulnlist.txt"
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
		-go-version=1.23.4 \
		-only-fixed \
		-dir="${dir}" \
		-nvd-api-key="$(cat "./api.text")" | tee "${dir}/vulnlist.txt"
	if [[ -z $(cat "${dir}/vulnlist.txt") ]]; then
		# Print this, otherwise error on the above might keep this file mistakenly empty.
		echo "no vulnerabilities" >"${dir}/vu
      }lnlist.txt"
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
	done <"${vuln_file}"
	echo "🔄 Resolving ${dir}/go.mod..."
	pushd "${dir}"
	go mod tidy
	popd
	rm "${dir}/go.mod.bak"
}

release-lib::idemp::git_commit_amend_match() {
	# Anything staged?
	if ! git diff-index --quiet --cached HEAD; then
		release-lib::git_commit_amend_match "${1}"
	fi
}

release-lib::git_commit_amend_match() {
	local message="${1}"
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

release-lib::needs_push() {
	local branch_to_push="${1}"
	if [[ -z "${branch_to_push}" ]]; then
		echo "❌  branch_to_push environment variable is not set." >&2
		return 1
	fi
	local base_branch_to_diff="${2}"
	if [[ -z "${base_branch_to_diff}" ]]; then
		echo "❌  base_branch_to_diff environment variable is not set." >&2
		return 1
	fi
	git checkout "${branch_to_push}"

	# TODO: Fix edge case - deleting branch remotely does not delete it locally (origin ref).
	if upstream_head=$(git fetch && git rev-parse "origin/${branch_to_push}"); then
		if [[ "$(git rev-parse HEAD)" == "${upstream_head}" ]]; then
			echo "⚠️ Nothing to push; origin/${branch_to_push} is all up to date"
			return 1
		fi
		git --no-pager log --oneline "${upstream_head}"...HEAD
		return 0
	fi
	# Likely "origin/${branch_to_push}" does not exists yet, so definitely something to
	# push (full $branch_to_push). Assuming the $branch_to_push will be proposed to be merged to
	# $base_branch_to_diff, so showing a full diff vs $base_branch_to_diff.
	if upstream_base_head=$(git fetch && git rev-parse "origin/${base_branch_to_diff}"); then
		if [[ "$(git rev-parse HEAD)" == "${upstream_base_head}" ]]; then
			echo "⚠️ Nothing to push, even the base origin/${base_branch_to_diff} is up to date; did you expect that?"
			return 1
		fi
		git --no-pager log --oneline "${upstream_base_head}"...HEAD
		return 0
	fi
}

# Extended regular expressions (ERE) regex matching paths to exclude from the fork commit.
# NOTE: # ^\..+ means all hidden files (e.g. changes to .golangci.yaml .gitignore or CI).
# TODO(bwplotka): Consider moving to globs with dotglob and extglob settings.. or Go (:
export EXCLUDE_RE="^\..+
^README\.md
^CHANGELOG\.md
^MAINTAINERS\.md
^CONTRIBUTING\.md
^RELEASE\.md
^Dockerfile
^docs/.*
^documentation/.*
^google/.*
^.*go\..*
^.*\.gitignore
^.*package.json
^.*package-lock.json
^Makefile.*
^.*vendor/.*
^VERSION
^.*node_modules/.*"

# Extended regular expressions (ERE) regex matching paths from EXCLUDE_RE that should be included.
# This is needed as it's simpler than implementing RE negative matchers.
# NOTE: For Prometheus the two specific documentation files are imported in Google Managed Prometheus docs, so keep those.
export DOCUMENTATION_INCLUDE_RE="^documentation/examples/prometheus-agent\.y.?ml
^documentation/examples/prometheus\.y.?ml"

release-lib::exclude_changes_from_last_commit() {
	local exclude_regexes=$1
	if [[ -z "${exclude_regexes}" ]]; then
		echo "❌  exclude_regexes is required." >&2
		return 1
	fi
	local include_regexes=$2
	if [[ -z "${include_regexes}" ]]; then
		echo "❌  include_regexes is required." >&2
		return 1
	fi
	local commit_title=$3
	if [[ -z "${commit_title}" ]]; then
		echo "❌  commit_title is required." >&2
		return 1
	fi

	# Get all files touched by a git commit, delimited by space.
	changed_files=$(git show --pretty="" --name-only "$(git rev-parse --verify HEAD)")
	if [ -z "${changed_files}" ]; then
		echo "❌  suspicious HEAD commit, no files changed." >&2
		return 1
	fi

	# Change to \n delimit (needed for grep to work) and exclude/include lines.
	tmp_to_exclude=$(echo "${changed_files}" | tr ' ' '\n' | grep -E "${exclude_regexes}" | grep -v -E "${include_regexes}")

	# Group node_module and vendor changes, we know we want to get rid of full directories here -- too many of those files slowing things down and obscuring the summary in git commit -- git restore supports globs.
	to_exclude=$(echo "${tmp_to_exclude}" | gsed -e 's|vendor/.*$|vendor/*|' -e 's|node_modules/.*$|node_modules/*|' | sort -u | tr ' ' '\n')
	if [ -z "${to_exclude}" ]; then
		# Nothing to exclude.
		return 0
	fi

	echo "🔄 Excluding the following files from the fork squash commit: ${to_exclude}; appending this information to the git commit message"
	curr_msg=$(git log --format=%B -n1)

	# Get all changes to be in stage area.
	git reset --soft HEAD~1
	while IFS= read -r exclude_path; do
		git restore -S "${exclude_path}"
	done <<<"${to_exclude}"
	# Commit after unstaging exclusions.
	# TODO(bwplotka): Handle nothing to commit after exclusion case.
	git commit -m "${commit_title}" -m "${curr_msg}" -m "Excluded files:
${to_exclude}
"
	git restore .
	git clean -fd
}
