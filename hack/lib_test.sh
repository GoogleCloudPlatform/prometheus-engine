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
source "${SCRIPT_DIR}/test-lib.sh"
source "${SCRIPT_DIR}/lib.sh"

TEST_FAILED=0
cleanup() {
	if ((TEST_FAILED)); then
		echo "${line_break}"
		echo "$test_name: FAILED"
		echo "${line_break}"
		# Print out the directories.
		echo "CHECKOUT_DIR=${CHECKOUT_DIR}"
	else
		rm -rf \
			"${CHECKOUT_DIR:-}"
	fi
}

exit_on_error() {
	if [ $? -ne 0 ]; then
		echo "Error!" >&2
		exit 1
	fi
}

setup() {
	CHECKOUT_DIR=$(mktemp --tmpdir -d tmpcheckout.XXXXX)
	export CHECKOUT_DIR
}

create_file() {
	mkdir -p "$(dirname $1)"
	echo "change" >$1
}
test_exclude_changes_from_last_commit() {
	setup

	# Add a fake state and branches to fake repo.
	git init "${CHECKOUT_DIR}"
	pushd "${CHECKOUT_DIR}"

	git commit --allow-empty -m "COMMIT 1: initial commit"

	create_file cmd/prometheus/main.go
	create_file config/config.go
	create_file documentation/examples/prometheus-agent.yml
	create_file documentation/examples/prometheus.yml
	create_file tsdb/some.go

	# Files expected to be excluded.
	create_file .github/workflows/some-ci.yaml
	create_file README.md
	create_file VERSION.md
	create_file docs/command-line/prometheus.md
	create_file documentation/examples/remote_storage/vendor/abc/d.go
	create_file documentation/examples/remote_storage/vendor/abc2/a.ini
	create_file go.mod
	create_file go.sum
	create_file vendor/abc2/a.ini
	create_file web/ui/node_modules/webpack/whatever.js

	git add --all
	git commit -m "some commit"

	release-lib::exclude_changes_from_last_commit "${RELEASE_LIB_EXCLUDE_RE}" "${RELEASE_LIB_DOCUMENTATION_INCLUDE_RE}" "COMMIT 2: my commit"
	exit_on_error

	got="$(git -C "${CHECKOUT_DIR}" log --format=%B --no-decorate)"
	expected=$(
		cat <<EOF
COMMIT 2: my commit

some commit

Excluded files:
.github/workflows/some-ci.yaml
README.md
VERSION.md
docs/command-line/prometheus.md
documentation/examples/remote_storage/vendor/*
go.mod
go.sum
vendor/*
web/ui/node_modules/*

COMMIT 1: initial commit
EOF
	)
	assert_equal "${got}" "${expected}"
}

run_all_tests() {
	run test_exclude_changes_from_last_commit
}

run() {
	local test_name=$1
	local line_break="========================================"
	echo "${line_break}"
	echo "=== Running $test_name"
	echo "${line_break}"
	# Assume test failure. The only way for the test to succeed if *all
	# statements* execute without errors and without exiting early due to some
	# other failure. If instead we assume that the test will pass and wait until
	# it fully returns to set TEST_FAILED=1 in an else clause, then that else
	# clause might never execute because the `trap ...` will be loaded onto the
	# stack first.
	TEST_FAILED=1
	# TODO(bwplotka): Bash gods are not happy; it seems this is a bit broken
	# as set +e is not applied to the sub function call. Either fix it or move to Go...
	if $test_name; then
		echo "${line_break}"
		echo "$test_name: OK"
		echo "${line_break}"
		echo
		TEST_FAILED=0
	fi
	cleanup
}
# Ensure the cleanup happens.
trap cleanup EXIT INT TERM
# Run tests.
run_all_tests
echo "ALL TESTS PASSED"
