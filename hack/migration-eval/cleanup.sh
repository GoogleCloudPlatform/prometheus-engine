#!/usr/bin/env bash
# Copyright 2024 Google LLC
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

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

if command -v kind >/dev/null 2>&1 && kind get clusters 2>/dev/null | grep -q "^gmp-eval$"; then
  echo "Deleting local Kind cluster 'gmp-eval'..."
  kind delete cluster --name gmp-eval
  echo "Kind cluster 'gmp-eval' deleted."
else
  echo "No local Kind cluster 'gmp-eval' found. Nothing to clean up."
fi

# Clean up built binary if present
rm -f "${REPO_ROOT}/bin/gmp-migrate"
