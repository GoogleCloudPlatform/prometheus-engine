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

# Compile the UI assets.
FROM --platform=$BUILDPLATFORM docker.io/node:22.11.0-bookworm@sha256:de4c8be8232b7081d8846360d73ce6dbff33c6636f2259cd14d82c0de1ac3ff2 AS assets
WORKDIR /app
COPY --from=docker.io/golang:1.23.3-bookwork@sha256:0e3377d7a71c1fcb31cdc3215292712e83baec44e4792aeaa75e503cfcae16ec /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY . /app
RUN pkg/ui/build.sh

# sync is used to copy all auto-generated files to a different context.
# Usually this is used to mirror the changes back to the host machine.
FROM scratch AS sync
COPY --from=assets /app/pkg/ui/embed.go pkg/ui/embed.go
COPY --from=assets /app/pkg/ui/static pkg/ui/static

# Build the actual Go binary.
FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.23.4@sha256:c419d4a81d5a735d9f480f8e848e07514e63ef2afed162abc571e85dd8cac882 AS buildbase
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY --from=assets /app ./
RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
      apt install -y --no-install-recommends \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
      CC=aarch64-linux-gnu-gcc; \
    fi && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=1 CC=${CC} \
      go build -tags boring,builtinassets -mod=vendor -o frontend cmd/frontend/*.go

FROM gcr.io/distroless/base-nossl-debian12:nonroot
COPY --from=buildbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
