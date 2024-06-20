# Copyright 2024 Google LLC
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

FROM golang:1.22-bullseye@sha256:067c5c7fe6d79f900c5ebe8351166356d6e3bbfcc6f807030e89b9a929252273 AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase

RUN CGO_ENABLED=0 go build -o go-synthetic ./examples/instrumentation/go-synthetic

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/go-synthetic /bin/go-synthetic
ENTRYPOINT ["/bin/go-synthetic"]
