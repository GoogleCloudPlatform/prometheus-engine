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

FORK_COMMIT_FILE=".git/fork-commit-sha.txt"

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

Create a new release of fork against an upstream version (tag) while carrying the patches from the
previous fork version. The process involves an "semantic fork git rebase -i" flow, similar to the normal git rebase -i (see semantic_fork_rebase for details).

NOTE: The script is idempotent; to force it to recreate local artifacts (e.g. local clones, remote branches it created), remove the artifact you want to recreate.

Example use:
* SOURCE_BRANCH=release-2.45.3-gmp UPSTREAM_TAG=v2.53.5 CHECKOUT_DIR=~/Repos/tmp-release bash ${me}

Variables:
* CHECKOUT_DIR (required) - Local working directory e.g. for local clones.
* SOURCE_BRANCH (required) - Fork branch considered as a source for the forked changes. This branch will be semantically rebased on UPSTREAM_TAG.
* UPSTREAM_TAG (required) - Upstream tag to synchronize (rebase) to; this also controls the name of the eventual branch to use for the sync (release-$UPSTREAM_TAG-gmp).
* PR_BRANCH (default: $USER/cut-release-$UPSTREAM_TAG-gmp) - Upstream branch to push to.
_EOM
}

if (($# > 0)); then
	case $1 in
	help)
		usage
		exit 1
		;;
	esac
fi

if [[ -z "${CHECKOUT_DIR}" ]]; then
	echo "❌  CHECKOUT_DIR environment variable is not set." >&2
	usage
	exit 1
fi
if [[ -z "${SOURCE_BRANCH}" ]]; then
	echo "❌  SOURCE_BRANCH environment variable is not set." >&2
	usage
	exit 1
fi
if [[ -z "${UPSTREAM_TAG}" ]]; then
	echo "❌  UPSTREAM_TAG environment variable is not set." >&2
	usage
	exit 1
fi

idemp::create_fork_commit() {
	if [[ ! -f "${FORK_COMMIT_FILE}" || -z $(cat "${FORK_COMMIT_FILE}") ]]; then
		create_fork_commit
	else
		echo "⚠️ Using existing ${FORK_COMMIT_FILE}"
	fi
}

# create_fork_commit prepare a squashed commit with the core fork changes to cherry-pick.
# It writes the SHA of that commit into the ${FORK_COMMIT_FILE}
#
# This commit excludes all files that are:
# * no longer needed (documentation)
# * can be easily recreated (e.g. by checking out the latest state or the automation like go mod vendor).
#
# Exclusion significantly simplifies the fork sync procedure.
create_fork_commit() {
	# Create ephemeral branch from the source of the forked code.
	git checkout "${SOURCE_BRANCH}"
	git checkout --detach

	# Get the exact upstream tag we base our fork on.
	source_tag="${SOURCE_BRANCH#release-}"
	source_tag="v${source_tag%-gmp}"

	# Reset it to clean vanilla version.
	git reset --hard "${source_tag}"

	# Squash all forked changes into a single commit.
	# HEAD@{1} is where the branch was just before the previous command.
	# TODO(bwplotka): Handle no change here.
	git merge --squash "HEAD@{1}" && git commit --no-edit

	# We could take it as-is but there are trivial changes we know we have to recreate, exclude them.
	release-lib::exclude_changes_from_last_commit "${RELEASE_LIB_EXCLUDE_RE}" "${RELEASE_LIB_DOCUMENTATION_INCLUDE_RE}" "google-patch[logic]: required upstream code modifications"
	git rev-parse --verify HEAD >"${FORK_COMMIT_FILE}"
	git checkout "${PR_BRANCH}"
}

force_clean_git_local_changes() {
	git restore -S .
	git restore .
	git clean -fd
}

idemp::recreate_fork_base_files() {
	pushd "${DIR}"
	if [[ "$(git rev-parse --verify HEAD)" != "$(git rev-list -n 1 "${UPSTREAM_TAG}")" ]]; then
		# There are some commits already, check if this step was done ~correctly.
		# Get 2 first commits from the upstream tag to HEAD.
		commits_state=$(git log --oneline --no-decorate "${UPSTREAM_TAG}"..HEAD | cut -d' ' -f2- | tail -2)
		expected="google-patch[libs]: add google fork packages (export and secret)
google-patch[setup]: initial setup"
		if [[ "${PROJECT}" != "prometheus" ]]; then
			expected="google-patch[setup]: initial setup"
			commits_state=$(echo "${commits_state}" | tail -1)
		fi
		if ! state_diff=$(diff <(echo "${commits_state}") <(echo "${expected}")); then
			echo "❌  Unexpected commit state on $(pwd); expected first commits to match
${expected}

diff:
${state_diff}

Remove those commits or remove the dir to recreate and rerun."
			exit 1
		fi
		echo "⚠️ Using existing commit state; looks clean:
${commits_state}"
		return 0
	fi
	if [ -n "$(git status --porcelain)" ]; then
		echo "❌  Unclean git state on $(pwd); clean it (e.g. git clean -fd) or remove the dir to recreate; and rerun."
		exit 1
	fi
	recreate_fork_base_files
}

# recreate_fork_base_files creates a few base commits:
# * setup (e.g. removal unnecessary files, documentation, gitignore, CI files, Dockerfile)
# * libs (e.g. any libs with non-conflicting code that we maintain on forks e.g. export pkg).
#
# NOTE: This is interconnected with $RELEASE_LIB_EXCLUDE_RE variable that excludes some of those files
# from cherry-pick to recreate it here.
recreate_fork_base_files() {
	echo "🔄 Creating 'google-patch[setup]' on top of the ${UPSTREAM_TAG} state..."

	# Remove unnecessary files, especially documentation - we want ppl to use GCP or upstream Prometheus docs.
	rm "MAINTAINERS.md"
	rm "CHANGELOG.md"
	rm "RELEASE.md"
	rm -r ".circleci/"
	rm -r ".github/"
	rm -r "docs/"
	# Remove all but important linked files.
	find "documentation" -type f | grep -v -E "${RELEASE_LIB_DOCUMENTATION_INCLUDE_RE}" | xargs rm --
	find "documentation" -type d -empty -delete

	git checkout "${SOURCE_BRANCH}" -- "README.md"
	git checkout "${SOURCE_BRANCH}" -- "CONTRIBUTING.md"
	git checkout "${SOURCE_BRANCH}" -- ".gcloudignore"

	# Apply our simplified CI.
	git checkout "${SOURCE_BRANCH}" -- ".github/"

	# Add our own build pipeline.
	# TODO(bwplotka): We could consider Dockerfile.google and removal of vanilla Dockerfile
	# This needs fix on Louhi side, so has to be carefully done.
	git checkout "${SOURCE_BRANCH}" -- "Dockerfile"

	git add --all
	git commit -s -m "google-patch[setup]: initial setup

Changes:
* Removal of unnecessary files (e.g. documentation) to avoid confusion.
* Replacing README and CONTRIBUTING doc files with our wording.
* Replacing CI scripts with our own.
* Replacing Dockerfile with our own for Google assured build.
* Adding .gcloudignore
"
	if [ -n "$(git status --porcelain)" ]; then
		echo "❌  Unclean git state on $(pwd); clean it (e.g. git clean -fd) or remove the dir to recreate; and rerun."
		exit 1
	fi
	if [[ "${PROJECT}" == "prometheus" ]]; then
		# After unfork this will change, but currently we have a separate package to maintain on Prometheus fork for:
		# * secrets
		# * lease
		# * GCM export
		# Recreate that instead of cherry-picking little commits.
		echo "🔄 Creating 'google-patch[libs]' on top of the 'google-patch[setup]' commit..."
		if ! git checkout "${SOURCE_BRANCH}" -- "google/"; then
			echo "❌  Expected google lib is missing on ${SOURCE_BRANCH}; Perhaps source branch is missing that (too old) or we are in progress of the unforking; double check and update script or fetch it manuall and commit with the 'google-patch[libs]:' prefix"
			exit 1
		fi

		git add --all
		git commit -s -m "google-patch[libs]: add google fork packages (export and secret)

  Changes:
  * Add google directory with export and secret code.
  "
	fi
	# All done, nothing to do.
}

REMOTE_URL=$(release-lib::remote_url_from_branch "${SOURCE_BRANCH}")
PROJECT=$(
	tmp=${REMOTE_URL##*/}
	echo "${tmp%.git}"
)
UPSTREAM_REMOTE_URL=$(release-lib::upstream_remote_url "${PROJECT}")
RELEASE_BRANCH="release-${UPSTREAM_TAG#v}-gmp"
PR_BRANCH=${PR_BRANCH:-"${USER}/cut-${RELEASE_BRANCH}"}

echo "🔄 Assuming ${PROJECT} with remotes {internal: ${REMOTE_URL}, upstream: ${UPSTREAM_REMOTE_URL}}; syncing ${UPSTREAM_TAG} into ${RELEASE_BRANCH} with the internal patches synced to ${PR_BRANCH} from ${SOURCE_BRANCH}"

DIR="${CHECKOUT_DIR}/${PROJECT}"
release-lib::idemp::clone "${DIR}" "${SOURCE_BRANCH}" "${PR_BRANCH}"

pushd "${DIR}"

if ! url=$(git remote get-url upstream 2>/dev/null) || [[ "${url}" != "${UPSTREAM_REMOTE_URL}" ]]; then
	git remote add upstream "${UPSTREAM_REMOTE_URL}"
fi

# Is cherry pick in progress?
if git rev-parse -q --verify CHERRY_PICK_HEAD; then
	echo "❌ Cherry pick is in progress on ${DIR}; cd there and fix conflicts manually, then --continue and rerun the script" >&2
	exit 1
fi

# Step 1: Prepare ${RELEASE_BRANCH} and ${PR_BRANCH}, both targeting vanilla ${UPSTREAM_TAG} for now. This will be used as a base for a PR with rebased forked functionality.
if ! git fetch upstream "refs/tags/${UPSTREAM_TAG}:refs/tags/${UPSTREAM_TAG}"; then
	echo "❌  Failed to fetch ${UPSTREAM_TAG} from the upstream ${UPSTREAM_REMOTE_URL}" >&2
	exit 1
fi

if git fetch origin && git rev-parse -q "origin/${RELEASE_BRANCH}" >/dev/null; then
	echo "❌  The internal 'origin/${RELEASE_BRANCH}' branch already exists. This means that this script was already run sucessfully or the release branch is live; aborting.

   Remove that remote branch and ${DIR} if you wish to recreate this branch." >&2
	exit 1
fi

if git rev-parse -q "${RELEASE_BRANCH}" >/dev/null; then
	release_branch_head=$(git rev-parse -q "${RELEASE_BRANCH}")
	if [[ "${release_branch_head}" != "$(git rev-list -n 1 "${UPSTREAM_TAG}")" ]]; then
		echo "❌  Internal '${RELEASE_BRANCH}' branch already exists and it's not pointing to the upstream ${UPSTREAM_TAG}; aborting. Remove that branch if you wish to resume this script." >&2
		exit 1
	fi
else
	git checkout "${UPSTREAM_TAG}" --detach
	git checkout -b "${RELEASE_BRANCH}"

	# Switch back to PR_BRANCH and reset it to RELEASE_BRANCH.
	git checkout "${PR_BRANCH}"
	git reset --hard "${RELEASE_BRANCH}"
fi

# Step 2: Prepare a squashed commit for core fork changes to cherry-pick. Exclude
# all files that are either no longer needed (documentation) or can be easily recreated
# (e.g. by checking out the latest state or the automation). This significantly simplifies
# the fork sync procedure.
if ! idemp::create_fork_commit "${DIR}"; then
	force_clean_git_local_changes
	git checkout "${PR_BRANCH}" # Ensure clean state.
	exit 1
fi

# Step 3: Recreate base fork commits like setup, build and vendor.
git checkout "${PR_BRANCH}"
idemp::recreate_fork_base_files

# Step 4: Cherry-pick the squashed fork commit -- this will likely conflict and manual intervention and review is needed.

if [[ "$(git log --oneline --no-decorate -n1 | cut -d' ' -f2- | cut -d':' -f1)" != "google-patch[logic]" ]]; then
	echo "🔄 Cherry picking prepared commit from the ${FORK_COMMIT_FILE}..."
	if ! git cherry-pick "$(cat "${FORK_COMMIT_FILE}")"; then
		echo "❌ Cherry pick found some conflicts (generally expected for this commit). Go to the directory, fix the issues and run 'git cherry-pick --continue', don't change the commit message (!); then rerun the script.  Dir:
  cd ${DIR}

  NOTE: You can run 'make test' to run all tests. After cherry-pick --continue, you can also do git rebase -i ${UPSTREAM_TAG} and arrange/change commits as you need." >&2
		exit 1
	fi
else
	echo "⚠️ google-patch[logic]* is present; assuming cherry-pick was done successfully; proceeding."
fi

if release-lib::needs_push "${PR_BRANCH}" "${RELEASE_BRANCH}"; then
	git --no-pager log --oneline "${RELEASE_BRANCH}"...HEAD

	if release-lib::confirm "About to git push state from ${DIR} to origin/${PR_BRANCH}; also pushing ${RELEASE_BRANCH} with the upstream state; are you sure?"; then
		git push origin "${PR_BRANCH}"
		git push origin "${RELEASE_BRANCH}"

		# TODO(bwplotka): Use gh to creat this
		echo "✅  Sync changes has been pushed on origin/${PR_BRANCH}; the vanilla upstream was pushed to ${RELEASE_BRANCH}; create a PR with origin/${PR_BRANCH} -> ${RELEASE_BRANCH}, ensure CI is passing and get the changes reviewed (especially  the cherry-picked 'google-patch[logic]:' commit!)."
		echo "ℹ️  After PR is merged, consider running:
* Vulnerability check/fix:
BRANCH=${RELEASE_BRANCH} CHECKOUT_DIR=${CHECKOUT_DIR} bash ${SCRIPT_DIR}/release-vulnfix.sh
* RC release creation:
BRANCH=${RELEASE_BRANCH} TAG=${UPSTREAM_TAG}-gmp.0-rc.0 CHECKOUT_DIR=${CHECKOUT_DIR} bash ${SCRIPT_DIR}/release-rc.sh"
	fi
else
	exit 1
fi
