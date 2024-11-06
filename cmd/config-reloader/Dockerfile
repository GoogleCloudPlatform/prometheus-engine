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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.23.2@sha256:335533a0cbaa2e5b0dc038ac7023418ac68c3bb22b8828fc354088bb0340019b AS buildbase
ARG TARGETOS
ARG TAGETARCH
WORKDIR /app
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor vendor
COPY cmd cmd
COPY pkg pkg

RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
      apt install -y --no-install-recommends \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
      CC=aarch64-linux-gnu-gcc; \
    fi && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=1 CC=${CC} \
      go build -tags boring -mod=vendor -o config-reloader -ldflags='-linkmode=external -extldflags=-static' cmd/config-reloader/*.go

FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20240307.00_p0@sha256:4f834e207f2721977094aeec4c9daee7032c5daec2083c0be97760f4306e4f88
COPY --from=buildbase /app/config-reloader /bin/config-reloader
ENTRYPOINT ["/bin/config-reloader"]
