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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.27.4@sha256:9ca562ec61672333bbf60fdccc6af039ff9aae1eeffecfc5402954674daadc5e AS buildbase

# Compile the UI assets.
FROM --platform=$BUILDPLATFORM us-central1-docker.pkg.dev/serverless-runtimes/google-24-full/runtimes/nodejs24@sha256:3d26abb4ab7c10b9aecd5dd901b7fc225f7d2f92effa94e7c485f2a12468208d AS assets
WORKDIR /app
# To build the UI we need a recent node version and the go toolchain.
COPY --from=buildbase /usr/bin/make /usr/bin/
COPY --from=buildbase /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY --chown=33:33 ./third_party/prometheus_ui ./third_party/prometheus_ui
COPY --chown=33:33 go.mod go.sum ./
COPY --chown=33:33 tools tools
COPY --chown=33:33 ./cmd/frontend ./cmd/frontend
COPY --chown=33:33 ./internal/promapi ./internal/promapi
COPY --chown=33:33 ./pkg/ui ./pkg/ui
COPY --chown=33:33 ./pkg/secutil ./pkg/secutil
RUN ./pkg/ui/build.sh

# sync is used to copy all auto-generated files to a different context.
# Usually this is used to mirror the changes back to the host machine.
FROM scratch AS sync
COPY --from=assets /app/pkg/ui/embed.go pkg/ui/embed.go
COPY --from=assets /app/pkg/ui/static pkg/ui/static

# Build the actual Go binary.
FROM buildbase AS appbase
WORKDIR /app
COPY go.mod go.sum ./
COPY hack/retry.sh hack/retry.sh
# Copy the Go vendor directory only if it exists. Vendor folder will automatically
# cause 'go build' to use -mod=vendor flag (otherwise -mod=mod is used).
COPY vendor* vendor
ENV GOMODCACHE=/gocache/mod
ENV GOCACHE=/gocache/build
ENV GOTOOLCHAIN=local
# Pre-download Go modules with retries before anything else is copied, so this
# layer is keyed on go.mod/go.sum only and is shared by every image build.
# proxy.golang.org occasionally resets streams mid-download and `go build` does
# not retry, which is the most common cause of flaky CI image builds. The build
# step then runs with GOPROXY=off so it fails loudly instead of using the network.
# The module cache deliberately lives in this layer rather than in a cache mount:
# BuildKit may garbage-collect cache mounts between `docker build` invocations
# (observed on GitHub runners), whereas an evicted layer is simply rebuilt.
# When vendor/ is populated, go build uses it and needs no module cache.
RUN if [ ! -d vendor ]; then ./hack/retry.sh go mod download; fi
# Install yq (used to read the version below) as a native host binary, before any
# target OS/arch is configured, so cross-arch builds do not try to cgo-compile it.
COPY tools tools
RUN --mount=type=cache,target=/gocache/build \
	CGO_ENABLED=0 GOBIN=/usr/local/bin ./hack/retry.sh go -C tools install github.com/mikefarah/yq/v4
COPY charts/values.global.yaml charts/values.global.yaml
ARG TARGETOS
ARG TARGETARCH
ARG BUILDARCH
ENV GOEXPERIMENT=boringcrypto
ENV CGO_ENABLED=1
ENV GOFIPS140=off
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
COPY --from=assets /app ./
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
	-tags builtinassets \
	-ldflags="-X github.com/prometheus/common/version.Version=$(yq '.version' charts/values.global.yaml) \
	-X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o frontend \
	cmd/frontend/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20261101.00_p0@sha256:52e1fd9180da347fc8d735c50368872b33d7e9505b09a70a861ebe57940bf0c6
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
