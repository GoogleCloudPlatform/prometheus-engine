#!/usr/bin/env bash

# Copyright 2021 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

echo "Generating CRD yamls"

which controller-gen || go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CRD_DIR=${SCRIPT_ROOT}/cmd/operator/deploy/operator
EXAMPLES_DIR=${SCRIPT_ROOT}/examples
CRD_TMP=$(mktemp -d)

# Split current crds.yaml into individual CRD files.
csplit --quiet -f ${CRD_TMP}/crd- -b "%02d.yaml" ${CRD_DIR}/crds.yaml "/---/+1" "{*}"

# Re-generate each CRD patch separately (limitation of controller-gen).
CRD_TMPS=$(find $CRD_TMP -iname '*.yaml' | sort)
for i in $CRD_TMPS; do
  b=$(basename ${i})
  dir=${i%.yaml}
  mkdir -p ${dir}
  mv $i ${dir}/$b
  controller-gen schemapatch:manifests=${dir} output:dir=${dir} paths=./pkg/operator/apis/...
done

# Merge and overwrite crds.yaml. Remove last line so we don't produce
# a final empty file that would make repeated runs of this script fail
CRD_TMPS=$(find $CRD_TMP -iname '*.yaml' | sort)
sed -s '$a---' $CRD_TMPS | sed -e '$ d' > ${CRD_DIR}/crds-tmp.yaml
cp ${CRD_DIR}/crds-tmp.yaml ${CRD_DIR}/crds.yaml
mv ${CRD_DIR}/crds-tmp.yaml ${EXAMPLES_DIR}/setup.yaml
