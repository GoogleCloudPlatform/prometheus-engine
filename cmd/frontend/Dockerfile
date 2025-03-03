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
FROM --platform=$BUILDPLATFORM docker.io/node:23.9.0-bookworm@sha256:c29271c7f2b4788fe9b90a7506d790dc8f2ff46132e1b70e71bf0c0679c8451c AS assets
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
FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.24.0@sha256:f0ed15de6a2c1d1e595b8b3dfaf2dc6af66a38c1891591c4903bd3e1b2e76518 AS buildbase
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
