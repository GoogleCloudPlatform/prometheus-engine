# Copyright 2023 Google LLC
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

FROM golang:1.20 AS build-env
WORKDIR /tmp/workdir

COPY . .
RUN cd cmd/ping && CGO_ENABLED=0 GOOS=linux go build
RUN cd cmd/pinger && CGO_ENABLED=0 GOOS=linux go build

FROM scratch

COPY --from=build-env /tmp/workdir/cmd/ping/ping /bin/ping
COPY --from=build-env /tmp/workdir/cmd/pinger/pinger /bin/pinger

CMD ["/bin/ping", "-latency=90%500ms,10%200ms", "-success-probability=65"]
