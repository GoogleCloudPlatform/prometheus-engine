#!/usr/bin/env bash

# Copyright 2022 Google LLC
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

set -e

echo $PROMETHEUS

PROMETHEUS=$( realpath "${PROMETHEUS}" )

if [ -n "${PROMETHEUS_COMPARE}" ]; then
  PROMETHEUS_COMPARE=$( realpath "${PROMETHEUS_COMPARE}" )
fi

pushd "$( dirname "${BASH_SOURCE[0]}" )"
trap 'kill 0' SIGTERM

rm -rf data1 data2 data3

echo "Starting example metrics application"

go run app/example_app.go 2>&1 | sed -e "s/^/[example-app] /" &

echo "Starting fake GCM endpoint"

go run server.go --latency=100ms 2>&1 | sed -e "s/^/[server] /" &

sleep 2

common_flags="\
--config.file=prometheus.yaml \
--export.debug.disable-auth \
--export.endpoint=localhost:10001 \
--export.label.project-id=gpe-test-1 \
--export.label.location=europe-west1-b \
--export.label.cluster=test-1 \
--storage.tsdb.min-block-duration=1h \
--storage.tsdb.retention=4h \
"

echo "Starting Prometheus "

${PROMETHEUS} \
  ${common_flags} \
  --storage.tsdb.path=data1 \
  --web.listen-address="0.0.0.0:9090" \
  2>&1 | sed -e "s/^/[prometheus] /" &

echo "Starting non-exporting Prometheus "

${PROMETHEUS} \
  ${common_flags} \
  --export.disable \
  --storage.tsdb.path=data2 \
  --web.listen-address="0.0.0.0:9091" \
  2>&1 | sed -e "s/^/[prometheus-noexport] /" &


if [ -n "${PROMETHEUS_COMPARE}" ]; then
  echo "Starting comparison Prometheus"
  
  ${PROMETHEUS_COMPARE} \
    ${common_flags} \
    --storage.tsdb.path=data3 \
    --web.listen-address="0.0.0.0:9092" \
    2>&1 | sed -e "s/^/[prometheus-compare] /" &
fi

wait
popd
