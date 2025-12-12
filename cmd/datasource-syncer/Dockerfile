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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.24.11@sha256:cafcfecdfc9ae0ab09e84613542f3a4da3f0b0eb4f61509f0d3614470a0e4b3c AS buildbase
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY . ./

RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
      apt install -y --no-install-recommends \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
      CC=aarch64-linux-gnu-gcc; \
    fi && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=1 CC=${CC} \
      go build -tags boring -mod=vendor -o datasource-syncer cmd/datasource-syncer/*.go

FROM gcr.io/distroless/base-nossl-debian12:nonroot
COPY --from=buildbase /app/datasource-syncer /bin/datasource-syncer
ENTRYPOINT ["/bin/datasource-syncer"]

