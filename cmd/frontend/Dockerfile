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
FROM --platform=$BUILDPLATFORM docker.io/node:23.11.0-bookworm@sha256:a9de312c8f40add3e897b9987e5d62eafcd39de785fe67dc3e70c4d2128153ec AS assets
WORKDIR /app
COPY --from=google-go.pkg.dev/golang:1.24.13@sha256:2ea380febc64f22355a66fd6141eb6575276b3ec45c97c1594f657bc47b60e60 /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY . /app
RUN pkg/ui/build.sh

# sync is used to copy all auto-generated files to a different context.
# Usually this is used to mirror the changes back to the host machine.
FROM scratch AS sync
COPY --from=assets /app/pkg/ui/embed.go pkg/ui/embed.go
COPY --from=assets /app/pkg/ui/static pkg/ui/static

# Build the actual Go binary.
FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.24.13@sha256:2ea380febc64f22355a66fd6141eb6575276b3ec45c97c1594f657bc47b60e60 AS buildbase
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
