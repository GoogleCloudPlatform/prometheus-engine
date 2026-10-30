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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.27.3@sha256:4d14dcb0974d4d5a608700a6a495e9730813582dadab5ceb4ff4b99d5462f897 AS buildbase
ARG TARGETOS
ARG TARGETARCH
ARG BUILDARCH
WORKDIR /app
COPY charts/values.global.yaml charts/values.global.yaml
COPY go.mod go.mod
COPY go.sum go.sum
COPY tools tools
COPY hack/retry.sh hack/retry.sh
# Copy the Go vendor directory only if it exists. Vendor folder will automatically
# cause 'go build' to use -mod=vendor flag (otherwise -mod=mod is used).
COPY vendor* vendor

ENV GOEXPERIMENT=boringcrypto
ENV CGO_ENABLED=1
ENV GOFIPS140=off
ENV GOTOOLCHAIN=local
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
ENV GOMODCACHE=/gocache/mod
ENV GOCACHE=/gocache/build
# Pre-download Go modules (and the yq tool used below) with retries, before the
# sources are copied so this layer is keyed on go.mod/go.sum only.
# proxy.golang.org occasionally resets streams mid-download and `go build` does
# not retry, which is the most common cause of flaky CI image builds. The build
# step then runs with GOPROXY=off so it fails loudly instead of using the network.
# The module cache deliberately lives in this layer rather than in a cache mount:
# BuildKit may garbage-collect cache mounts between `docker build` invocations
# (observed on GitHub runners), whereas an evicted layer is simply rebuilt.
RUN --mount=type=cache,target=/gocache/build \
	./hack/retry.sh go mod download && \
	./hack/retry.sh go tool -modfile="tools/go.mod" yq --version
COPY cmd/gmp-migrate cmd/gmp-migrate
COPY pkg pkg
RUN --mount=type=cache,target=/gocache/build \
	--mount=type=cache,target=/var/cache/apt,sharing=locked \
	--mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
	if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
	rm -f /etc/apt/apt.conf.d/docker-clean && \
	apt-get update && apt-get install -y --no-install-recommends \
	gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
	export CC=aarch64-linux-gnu-gcc; \
	elif [ "${TARGETARCH}" = "amd64" ] && [ "${BUILDARCH}" != "amd64" ]; then \
	rm -f /etc/apt/apt.conf.d/docker-clean && \
	apt-get update && apt-get install -y --no-install-recommends \
	gcc-x86-64-linux-gnu libc6-dev-amd64-cross; \
	export CC=x86_64-linux-gnu-gcc; \
	fi && \
	export GOPROXY=off && \
	GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build \
	-ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | go tool -modfile="tools/go.mod" yq '.version' ) \
	-X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o gmp-migrate \
	cmd/gmp-migrate/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20260928.00_p0@sha256:521379cbafae8f38a3cc396b9abdc569872ff9c4e26e7f6dee7b564b4fd33ab3
COPY --from=buildbase /app/gmp-migrate /bin/gmp-migrate
ENTRYPOINT ["/bin/gmp-migrate"]
