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
	return 1
fi

source "${SCRIPT_DIR}/lib.sh"

# TODO: Find better way. Go tool grane is tricky as we run in different directory.
go install github.com/google/go-containerregistry/cmd/gcrane@latest

# Also accepts SYNC_DOCKERFILES_FROM.

if [[ -z "${DIR}" ]]; then
	log_err "DIR envvar is required."
	exit 1
fi

if [[ -z "${BRANCH}" ]]; then
	log_err "BRANCH envvar is required."
	exit 1
fi

if [[ -z "${PROJECT}" ]]; then
	log_err "PROJECT envvar is required."
	exit 1
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
	exit 1
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
	git add --all
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
		git add --all
	fi

	# Check if that helped.
	echo "⚠️  This will fail on older branches with vendoring; in this case, simply go to ${DIR}, run 'go mod vendor' and rerun."
	release-lib::vulnlist "${DIR}" "${vuln_file}"
	if [[ "no vulnerabilities" != $(cat "${vuln_file}") ]]; then
		echo "❌  After go mod update some vulnerabilities are still found; go to ${DIR} and resolve it manually (select not reusing the ./vulnlist.txt file) and rerun."
		exit 1
	fi
fi
