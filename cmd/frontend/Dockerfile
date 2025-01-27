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

# TODO(bwplotka): Move to 1.23 once ARM QEMU building is fixed https://github.com/golang/go/issues/68976
FROM golang:1.23.5@sha256:8c10f21bec412f08f73aa7b97ca5ac5f28a39d8a88030ad8a339fd0a781d72b4 AS buildbase

# Compile the UI assets.
FROM launcher.gcr.io/google/nodejs as assets
# To build the UI we need a recent node version and the go toolchain.
RUN install_node v17.9.0
COPY --from=buildbase /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY . /app
RUN pkg/ui/build.sh

# sync is used to copy all auto-generated files to a different context.
# Usually this is used to mirror the changes back to the host machine.
FROM scratch as sync
COPY --from=assets /app/pkg/ui/embed.go pkg/ui/embed.go
COPY --from=assets /app/pkg/ui/static pkg/ui/static

# Build the actual Go binary.
FROM buildbase AS appbase
WORKDIR /app
COPY --from=assets /app ./
RUN CGO_ENABLED=0 GOEXPERIMENT=boringcrypto go build -tags builtinassets -mod=vendor -o frontend ./cmd/frontend/*.go

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
