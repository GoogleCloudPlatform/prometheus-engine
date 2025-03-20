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

FROM golang:1.24-bullseye@sha256:3c669c8fed069d80d199073b806243c4bf79ad117b797b96f18177ad9c521cff AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 go build -mod=vendor -o datasource-syncer cmd/datasource-syncer/*.go

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/datasource-syncer /bin/datasource-syncer
ENTRYPOINT ["/bin/datasource-syncer"]