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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.25.4@sha256:3f90cabe0630565f75de1f7cdb0898bf9e39ff4b52df1d1e21aff75b87e68fde AS buildbase
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
COPY pkg pkg
COPY internal internal

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
    VERSION=$(awk -F': ' '/^version:/ {print $2}' charts/values.global.yaml) && \
    BUILD_DATE=$(date --iso-8601=seconds) && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CC=${CC} \
      go build \
        -ldflags="-X github.com/prometheus/common/version.Version=${VERSION} -X github.com/prometheus/common/version.BuildDate=${BUILD_DATE}" \
	-o rule-evaluator \
	cmd/rule-evaluator/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20250807.00_p0
COPY --from=buildbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
