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
WORKDIR /app
COPY go.mod go.sum ./
# Copy the Go vendor directory only if it exists. Vendor folder will automatically
# cause 'go build' to use -mod=vendor flag (otherwise -mod=mod is used).
COPY vendor* vendor
COPY pkg/secutil pkg/secutil
COPY examples/instrumentation/go-synthetic examples/instrumentation/go-synthetic

FROM buildbase AS appbase
ARG TARGETOS
ARG TARGETARCH
ARG BUILDARCH

ENV GOEXPERIMENT=boringcrypto
ENV CGO_ENABLED=1
ENV GOFIPS140=off
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
ENV GOMODCACHE=/gocache/mod
ENV GOCACHE=/gocache/build
RUN --mount=type=cache,target=/gocache/mod \
	--mount=type=cache,target=/gocache/build \
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
	GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build -o go-synthetic ./examples/instrumentation/go-synthetic

FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20260928.00_p0@sha256:521379cbafae8f38a3cc396b9abdc569872ff9c4e26e7f6dee7b564b4fd33ab3
COPY --from=appbase /app/go-synthetic /bin/go-synthetic
ENTRYPOINT ["/bin/go-synthetic"]
