#!/usr/bin/env bash

# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Retries a command with exponential back-off. Used by Dockerfiles and CI
# steps that download from proxy.golang.org or other flaky remotes.
#
# Usage: hack/retry.sh <command> [args...]
# Env:   RETRY_ATTEMPTS  number of attempts (default 5).
#        RETRY_DELAY     initial delay in seconds, doubled after each failure (default 5).

set -o errexit
set -o nounset
set -o pipefail

attempts=${RETRY_ATTEMPTS:-5}
delay=${RETRY_DELAY:-5}

for ((i = 1; i <= attempts; i++)); do
	if "$@"; then
		exit 0
	fi
	if ((i == attempts)); then
		echo "Command '$*' failed after ${attempts} attempts" >&2
		exit 1
	fi
	echo "Command '$*' failed (attempt ${i}/${attempts}); retrying in ${delay}s..." >&2
	sleep "${delay}"
	delay=$((delay * 2))
done
