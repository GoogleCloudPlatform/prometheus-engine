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
#
# See README.md#bash for rules to writing bash.

set -o errexit
set -o pipefail
set -o nounset

if [[ -n "${DEBUG_MODE:-}" ]]; then
	set -o xtrace
fi

log_err() {
	echo "❌  ${1}" >&2
}

# TODO(bwplotka): Finding correct script dir is not so trivial.
if [[ -z "${SCRIPT_DIR}" ]]; then
	log_err "SCRIPT_DIR envvar is required."
	return 1
fi

release-lib::confirm() {
	local prompt_message="${1:-Are you sure?}"

	# -p: Display the prompt string.
	# -r: Prevents backslash interpretation.
	# -n 1: Read only one character.
	read -p "$prompt_message [y/n/CTR+C]: " -r -n 1 response
	echo # Ensures the cursor moves to the next line after input.
	case "$response" in
	[yY])
		return 0
		;;
	[nN])
		log_err "The action has been cancelled as requested."
		return 1
		;;
	*)
		log_err "Invalid input. Exiting script."
		exit 1
		;;
	esac
}

release-lib::idemp::vulnlist() {
	local dir="${1}"
	if [[ -z "${dir}" ]]; then
		log_err "dir arg is required."
		return 1
	fi
	local vuln_file="${2}"
	if [[ -z "${vuln_file}" ]]; then
		log_err "vuln_file arg is required."
		return 1
	fi
	if [[ "${vuln_file}" != /* ]]; then
		log_err "vuln_file arg must point to an absolute file path."
		return 1
	fi

	# TODO(bwplotka): We could ask user if we should reuse existing vulnfile.
	release-lib::vulnlist "${dir}" "${vuln_file}"
}

release-lib::vulnlist() {
	local dir="${1}"
	if [[ -z "${dir}" ]]; then
		log_err "dir arg is required."
		return 1
	fi
	local vuln_file="${2}"
	if [[ -z "${vuln_file}" ]]; then
		log_err "vuln_file arg is required."
		return 1
	fi
	if [[ "${vuln_file}" != /* ]]; then
		log_err "vuln_file arg must point to an absolute file path."
		return 1
	fi

	readarray -t DOCKERFILES < <(release-lib::dockerfiles "${dir}")
	local go_version=$(release-lib::dockerfile_go_version "${DOCKERFILES[0]}")
	if [[ -z "${go_version}" ]]; then
		log_err "can't find any golang image in ${DOCKERFILES[0]}"
		return 1
	fi

	echo "🔄  Detecting Go ${go_version} vulnerabilities to fix..."
	pushd "${SCRIPT_DIR}/vulnupdatelist/"
	if [[ ! -f "./api.text" ]]; then
		log_err "$(pwd)/api.text file not found in your filesystem. Please create it with your NVD API key. See https://nvd.nist.gov/developers/request-an-api-key"
		return 1
	fi

	go run "./..." \
		-go-version=${go_version} \
		-only-fixed \
		-dir="${dir}" \
		-nvd-api-key="$(cat "./api.text")" | tee "${vuln_file}"
	if [[ -z $(cat "${vuln_file}") ]]; then
		# Print this, otherwise error on the above might keep this file mistakenly empty.
		echo "no vulnerabilities" >"${vuln_file}"
	fi
	popd
}

release-lib::gomod_vulnfix() {
	local dir=${1}
	if [[ -z "${dir}" ]]; then
		log_err "dir arg is required."
		return 1
	fi

	local vuln_file="${2}"
	if [[ -z "${vuln_file}" ]]; then
		log_err "vuln_file arg is required."
		return 1
	fi
	if [[ ! -f "${vuln_file}" ]]; then
		log_err "no ${vuln_file} file found"
		return 1
	fi

	if [[ "no vulnerabilities" == $(cat "${vuln_file}") ]]; then
		log_err "${vuln_file} shows no vulnerabilities"
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
		gsed -i "s|\(	${mod_path} \).*|\1${desired_version}|" "${dir}/go.mod"
	done <"${vuln_file}"
	echo "🔄 Resolving ${dir}/go.mod..."
	pushd "${dir}"
	go mod tidy
	popd
}

# Also accepts SYNC_DOCKERFILES_FROM.
function release-lib::vulnfix() {
	if [[ -z "${DIR}" ]]; then
		log_err "DIR envvar is required."
		return 1
	fi

	if [[ -z "${BRANCH}" ]]; then
		log_err "BRANCH envvar is required."
		return 1
	fi

	if [[ -z "${PROJECT}" ]]; then
		log_err "PROJECT envvar is required."
		return 1
	fi

	echo "${DIR}"
	echo "${SCRIPT_DIR}"

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
		return 1
	fi

	# TODO: git add charts & vendor for old projects?

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
	# TODO(bwplotka): Find better place to put this?
	mkdir -p "${DIR}/.gmpctl/"
	echo "*" >>"${DIR}/.gmpctl/.gitignore"
	vuln_file="${DIR}/.gmpctl/vulnlist.txt"
	pushd "${DIR}"

	release-lib::idemp::vulnlist "${DIR}" "${vuln_file}"

	if [[ "no vulnerabilities" != $(cat "${vuln_file}") ]]; then
		# Attempt to update + go mod tidy.
		release-lib::gomod_vulnfix "${DIR}" "${vuln_file}"
		git add go.mod go.sum

		if [ -d "${DIR}/vendor" ]; then
			go mod vendor
			git add --all # TODO: Can be flaky.
		fi

		# Check if that helped.
		echo "⚠️  This will fail on older branches with vendoring; in this case, simply go to ${DIR}, run 'go mod vendor' and rerun."
		release-lib::vulnlist "${DIR}" "${vuln_file}"
		if [[ "no vulnerabilities" != $(cat "${vuln_file}") ]]; then
			echo "❌  After go mod update some vulnerabilities are still found; go to ${DIR} and resolve it manually (select not reusing the ./vulnlist.txt file) and rerun."
			exit 1
		fi
	fi
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
		log_err "message is required."
		return 1
	fi
	if [[ "$(git log -1 --pretty=%s)" == "${message}" ]]; then
		git commit -s --amend -m "${message}"
	else
		git commit -sm "${message}"
	fi
}

# TODO(bwplotka): Move params to envvars for consistency.
release-lib::needs_push() {
	local branch_to_push="${1}"
	if [[ -z "${branch_to_push}" ]]; then
		log_err "branch_to_push environment variable is not set."
		return 1
	fi
	local base_branch_to_diff="${2}"
	if [[ -z "${base_branch_to_diff}" ]]; then
		log_err "base_branch_to_diff environment variable is not set."
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

release-lib::dockerfiles() {
	local dir="${1}"
	if [[ -z "${dir}" ]]; then
		log_err "dir arg is required."
		return 1
	fi
	find "${dir}" -name "Dockerfile*" | grep -v "${dir}/third_party/" | grep -v "${dir}/examples/" | grep -v "${dir}/hack/" | grep -v "${dir}/ui/"
}

# Return all images used in a Dockerfile, delimited by new-line.
# Use readarray if you need bash array:
# readarray -t IMAGES < <(release-lib::dockerfile_images_used "${dockerfile}")
release-lib::dockerfile_images_used() {
	local dockerfile=${1}
	if [[ -z "${dockerfile}" ]]; then
		log_err "dir arg is required."
		return 1
	fi

	if [ ! -f "${dockerfile}" ]; then
		log_err "File not found: $DOCKERFILE"
		return 1
	fi

	# FROM based image references e.g.: FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.25.1@sha256:2a5741ae38c60d188ae9144d1b63730b8059d016216e785b9b35acf1ace69bc1 AS buildbase
	grep '^FROM ' "${dockerfile}" |
		gsed -e 's/^FROM //i' \
			-e 's/--platform=[^ ]* //' \
			-e 's/ AS .*$//i' |
		while read -r full_image_string; do
			# NOTE: We miss images like launcher.gcr.io/google/nodejs but that's fine, let's fix it by
			# adding tag.
			echo "${full_image_string}" | grep -E "^.*(:|@).*$" || true
		done

	# Arg based image references e.g.: ARG IMAGE_BASE_DEBUG=gcr.io/distroless/base-nossl-debian12:debug
	grep '^ARG ' "${dockerfile}" |
		gsed -e 's/^ARG [^ ]*=//i' |
		while read -r full_image_string; do
			# NOTE: We miss images like launcher.gcr.io/google/nodejs but that's fine, let's fix it by
			# adding tag.
			echo "${full_image_string}" | grep -E "^.*(:|@).*$" || true
		done

	return 0
}

# TODO(bwplotka): Move params to envvars for consistency.
release-lib::dockerfile_go_version() {
	local dockerfile=${1}
	if [[ -z "${dockerfile}" ]]; then
		log_err "dir arg is required."
		return 1
	fi

	if [ ! -f "${dockerfile}" ]; then
		log_err "File not found: $DOCKERFILE"
		return 1
	fi

	for image in $(release-lib::dockerfile_images_used "${dockerfile}"); do
		# Initialize variables for each line
		image_name=""
		tag=""
		sha=""

		# --- 1. Extract SHA ---
		# Check if the string contains a SHA digest (delimited by '@')
		if [[ "$image" == *@* ]]; then
			# Use cut to split the string at the '@'
			image_and_tag=$(echo "$image" | cut -d'@' -f1)
			sha=$(echo "$image" | cut -d'@' -f2)
		else
			# No SHA found
			image_and_tag="$image"
			sha="<none>"
		fi

		# --- 2. Extract Tag ---
		# A tag is the part after the *last* colon.
		# We must check that this part doesn't contain a '/',
		# which would mean it's part of a port number (e.g., localhost:5000/my-image)

		# Get the part after the last colon
		last_part="${image_and_tag##*:}"

		if [[ "$last_part" == "$image_and_tag" || "$last_part" == */* ]]; then
			# Case 1: No colon found (e.g., "alpine")
			#    Here, last_part == image_and_tag
			# Case 2: Colon is part of a port/path (e.g., "my.registry:5000/image")
			#    Here, last_part == "5000/image", which matches */*
			image_name="$image_and_tag"
			tag="<none>"
		else
			# Case 3: A valid tag was found (e.g., "alpine:latest")
			#    Here, last_part == "latest"
			image_name="${image_and_tag%:*}"
			tag="$last_part"
		fi

		if [[ "${image_name}" == "google-go.pkg.dev/golang" ]]; then
			go_tag="${tag}"
			echo "${go_tag}"
			return 0
		fi
	done

	log_err "could not find golang image in Dockerfile: ${dockerfile}"
	return 1
}

# TODO(bwplotka): Move params to envvars for consistency.
release-lib::dockerfile_update_image() {
	local dockerfile=${1}
	if [[ -z "${dockerfile}" ]]; then
		log_err "dockerfile arg is required."
		return 1
	fi

	if [ ! -f "${dockerfile}" ]; then
		log_err "File not found: $DOCKERFILE"
		return 1
	fi

	local image=${2}
	if [[ -z "${image}" ]]; then
		log_err "image arg is required."
		return 1
	fi

	local tag_prefix=${3}
	if [[ -z "${tag_prefix}" ]]; then
		log_err "tag_prefix arg is required."
		return 1
	fi

	# Prerequisite tool.
	go install github.com/google/go-containerregistry/cmd/gcrane@latest

	# Use gcrane vs crane for --json.
	local all_tags=$(gcrane ls "${image}" --json | jq --raw-output '.tags[]' | sort -V)
	# Exclude RC images.
	all_tags=$(echo "${all_tags}" | grep -v "rc.*")
	# Prefix allows sticking to e.g. latest minor.
	all_tags=$(echo "${all_tags}" | grep "${tag_prefix}")
	local latest_tag=$(echo "${all_tags}" | tail -n1)

	local latest_digest=$(crane digest "${image}:${latest_tag}")
	local latest_image="${image}:${latest_tag}@${latest_digest}"

	echo "🔄  Ensuring ${latest_image} on ${dockerfile}..."
	for img in $(release-lib::dockerfile_images_used "${dockerfile}"); do
		if ! [[ "${img}" =~ ${image}* ]]; then
			continue
		fi

		# Found Go image, perform sed.
		gsed -i "s#${img}#${latest_image}#g" "${dockerfile}"
	done

	# TODO: Validate if swap was done? ::idemp?
	return 0
}

# TODO(bwplotka): Move params to envvars for consistency.
release-lib::idemp::manifests_bash_image_bump() {
	local dir=${1}
	if [[ -z "${dir}" ]]; then
		log_err "dir arg is required."
		return 1
	fi

	local values_file="${dir}/charts/values.global.yaml"
	# TODO: Not enough, this has to check actual manifests.
	local bash_tag=$(go tool yq '.images.bash.tag' "${values_file}")

	# Use gcrane vs crane for --json.
	local latest_bash_tag=$(go tool gcrane ls "gke.gcr.io/gke-distroless/bash" --json | jq --raw-output '.tags[]' | grep "gke_distroless_" | sort -V | tail -n1)
	if [[ "${bash_tag}" == "${latest_bash_tag}" ]]; then
		echo "✅  Nothing to do; ${values_file} already uses ${latest_bash_tag}"
		return 0
	fi

	# Upgrade.
	echo "🔄  Ensuring ${latest_bash_tag} on ${values_file}..."
	if ! gsed -i -E "s#tag: ${bash_tag}#tag: ${latest_bash_tag}#g" "${values_file}"; then
		# TODO: This is flaky, no failing actually on no match. Common bug is
		log_err "sed didn't replace?"
		return 1
	fi

	# Regen only what's needed.
	release-lib::manifests_regen "${dir}"
	echo "✅  Done!"
	return 0
}

# TODO(bwplotka): Move params to envvars for consistency.
release-lib::manifests_regen() {
	local dir=${1}
	if [[ -z "${dir}" ]]; then
		log_err "dir arg is required."
		return 1
	fi

	# TODO(bwplotka): Manage deps better. It's getting confusing what bins we should use (worktree bingo? script bingo?).
	# bingo get is sort of necessary here?
	source "${dir}/.bingo/variables.env"
	YQ="${YQ:-}" HELM="${HELM}" ADDLICENSE="${ADDLICENSE:-}" bash "${dir}/hack/presubmit.sh" manifests
	echo "✅  Manifests regenerated"
	return 0
}

# Also accepts "FORCE_NEW_PATCH_VERSION" envvar.
# TODO(bwplotka): Move params to envvars for consistency.
release-lib::next_release_tag() {
	local dir=${1}
	if [[ -z "${dir}" ]]; then
		log_err "dir arg is required."
		return 1
	fi

	pushd "${dir}" >&2

	# Get the latest tag from the current branch's history
	# `git describe --tags --abbrev=0` finds the closest tag in the ancestry.
	local LATEST_TAG=""
	if ! LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null); then
		log_err "No reachable tags found on this branch's history."
		echo "Please ensure you have tags and have fetched them." >&2
		return 1
	fi

	# Apply bumping logic.
	NEW_TAG=""
	if [[ "${LATEST_TAG}" == *"-rc."* && -z "${FORCE_NEW_PATCH_VERSION:-}" ]]; then
		# Get the part before "-rc." (e.g., "v1.2.3")
		BASE_VERSION="${LATEST_TAG%-rc.*}"
		# Get the part after "-rc." (e.g., "4")
		RC_NUMBER="${LATEST_TAG##*-rc.}"
		# Increment the RC number
		NEW_RC_NUMBER=$((RC_NUMBER + 1))

		NEW_TAG="${BASE_VERSION}-rc.${NEW_RC_NUMBER}"
	else
		# Preserve the 'v' prefix if it exists
		PREFIX=""
		if [[ "$LATEST_TAG" == v* ]]; then
			PREFIX="v"
		fi
		# Remove 'v' prefix for parsing (e.g., "1.2.3")
		VERSION_ONLY="${LATEST_TAG#v}"
		# Remove rc suffix, if exists.
		VERSION_ONLY="${VERSION_ONLY%-rc.*}"

		# Read major, minor, and patch into variables
		# We use a default of 0 for missing components
		IFS='.' read -r major minor patch <<<"$VERSION_ONLY"
		major=${major:-0}
		minor=${minor:-0}
		patch=${patch:-0}

		# Check that the patch version is a valid number
		if ! [[ "$patch" =~ ^[0-9]+$ ]]; then
			log_err "Latest tag '$LATEST_TAG' does not have a numeric patch version (x.y.Z)."
			return 1
		fi
		# Increment the patch number
		NEW_PATCH=$((patch + 1))
		NEW_TAG="${PREFIX}${major}.${minor}.${NEW_PATCH}-rc.0"
	fi

	popd >&2

	echo "${NEW_TAG}"
	return 0
}

function release-lib::pre-release-rc() {
	if [[ -z "${DIR}" ]]; then
		log_err "DIR envvar is required."
		return 1
	fi

	if [[ -z "${BRANCH}" ]]; then
		log_err "BRANCH envvar is required."
		return 1
	fi

	if [[ -z "${TAG}" ]]; then
		log_err "TAG envvar is required."
		return 1
	fi

	if [[ -z "${PROJECT}" ]]; then
		log_err "PROJECT envvar is required."
		return 1
	fi

	if [[ "${PROJECT}" == "prometheus-engine" ]]; then
		local CLEAN_TAG="${TAG%-rc.*}"
		CLEAN_TAG="${CLEAN_TAG#v}"
		if [[ "${BRANCH}" == "release/0.12" ]]; then
			# A bit different flow.
			local chart_file="${DIR}/charts/operator/Chart.yaml"
			echo "🔄  Ensuring ${CLEAN_TAG} on ${chart_file}..."
			if ! gsed -i -E "s#appVersion:.*#appVersion: ${CLEAN_TAG}#g" "${chart_file}"; then
				# TODO: This is flaky, no failing actually on no match. Common bug is
				echo "❌  sed didn't replace?"
				return 1
			fi

			chart_file="${DIR}/charts/rule-evaluator/Chart.yaml"
			echo "🔄  Ensuring ${CLEAN_TAG} on ${chart_file}..."
			if ! gsed -i -E "s#appVersion:.*#appVersion: ${CLEAN_TAG}#g" "${chart_file}"; then
				# TODO: This is flaky, no failing actually on no match. Common bug is
				echo "❌  sed didn't replace?"
				return 1
			fi
		else
			# 0.12+
			local values_file="${DIR}/charts/values.global.yaml"
			echo "🔄  Ensuring ${CLEAN_TAG} on ${values_file}..."
			if ! gsed -i -E "s#version:.*#version: ${CLEAN_TAG}#g" "${values_file}"; then
				# TODO: This is flaky, no failing actually on no match. Common bug is
				echo "❌  sed didn't replace?"
				return 1
			fi
		fi
		# For versions with export embedded.
		if [[ -f "${DIR}/pkg/export/export.go" ]]; then
			echo "🔄  Ensuring ${TAG} in ${DIR}/pkg/export/export.go mainModuleVersion..."
			if ! gsed -i -E "s#mainModuleVersion = .*#mainModuleVersion = \"${TAG}\"#g" "${DIR}/pkg/export/export.go"; then
				# TODO: This is flaky, no failing actually on no match. Common bug is
				echo "❌  sed didn't replace?"
				return 1
			fi
		fi

		release-lib::manifests_regen "${DIR}"
		git add --all
	else
		# Prometheus and Alertmanager fork needs just a correct version in the VERSION file,
		# so the binary build (go_build_info) metrics and flags are correct.
		local temp=${TAG#v} # Remove v and then -rc.* suffix.
		echo "${temp%-rc.*}" >VERSION
		git add VERSION
	fi
}
