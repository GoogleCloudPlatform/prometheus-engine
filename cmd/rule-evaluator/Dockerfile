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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.5@sha256:5075e8b0e1a7913c2274737c1a8b379ea264322c082bd21d3911d196dce46559 AS buildbase
ARG TARGETOS
ARG TARGETARCH
ARG BUILDARCH
WORKDIR /app
COPY charts/values.global.yaml charts/values.global.yaml
COPY go.mod go.mod
COPY go.sum go.sum
COPY tools tools
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
	apt-get update && apt-get install -y --no-install-recommends \
	gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
	export CC=aarch64-linux-gnu-gcc; \
	elif [ "${TARGETARCH}" = "amd64" ] && [ "${BUILDARCH}" != "amd64" ]; then \
	apt-get update && apt-get install -y --no-install-recommends \
	gcc-x86-64-linux-gnu libc6-dev-amd64-cross; \
	export CC=x86_64-linux-gnu-gcc; \
	fi && \
	GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build \
	-ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | go tool -modfile="tools/go.mod" yq '.version' ) \
	-X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o rule-evaluator \
	cmd/rule-evaluator/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20260715.00_p0@sha256:806b48f065651c95ef6547c4b136f0a1acfe1ad259bdae77c3696a60a044394f
COPY --from=buildbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
