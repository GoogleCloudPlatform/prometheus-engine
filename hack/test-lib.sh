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

_assert_equal() {
	got="${1}"
	expected="${2}"

	if [[ "${got}" == "${expected}" ]]; then
		return
	fi

	echo "assertion failure:"
	cat <<EOF
<<<<<<< GOT
${got}
=======
${expected}
>>>>>>> EXPECTED
EOF

	return 1
}

_assert_pass() {
	if eval "$1"; then
		return
	fi

	return 1
}

_assert_fail() {
	if ! eval "$1"; then
		return
	fi

	return 1
}

# Sanity check on pass/fail helpers.
_assert_pass true || exit 1
_assert_fail false || exit 1

_assert_fail "_assert_pass false" || exit 1
_assert_fail "_assert_fail true" || exit 1

_assert_pass "_assert_equal 1 1" || exit 1
echo "NOTE: Expected failure output below on _assert_fail (exit code 0 though)"
_assert_fail "_assert_equal 1 2" || exit 1

# Stronger assertion helpers that exit the program on assertion failure.
assert_pass() { _assert_pass "$@" || exit 1; }
assert_fail() { _assert_fail "$@" || exit 1; }
assert_equal() { _assert_equal "$@" || exit 1; }
