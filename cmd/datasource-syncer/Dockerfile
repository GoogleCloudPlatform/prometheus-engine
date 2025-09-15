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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.25.1@sha256:2a5741ae38c60d188ae9144d1b63730b8059d016216e785b9b35acf1ace69bc1 AS buildbase
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY charts/values.global.yaml charts/values.global.yaml
COPY go.mod go.mod
COPY go.sum go.sum
# Copy the Go vendor directory only if it exists. Vendor folder will automatically
# cause 'go build' to use -mod=vendor flag (otherwise -mod=mod is used).
COPY vendor* vendor
COPY cmd cmd

RUN GO111MODULE=on go get github.com/mikefarah/yq/v3 # Install yq tool.

ENV GOEXPERIMENT=boringcrypto
ENV CGO_ENABLED=1
ENV GOFIPS140=off
ENV GOTOOLCHAIN=local
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
      apt install -y --no-install-recommends \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
      CC=aarch64-linux-gnu-gcc; \
    fi && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CC=${CC} \
      go build \
        -ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | yq '.version' ) \
        -X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o datasource-syncer \
	cmd/datasource-syncer/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20250807.00_p0
COPY --from=buildbase /app/datasource-syncer /bin/datasource-syncer
ENTRYPOINT ["/bin/datasource-syncer"]

