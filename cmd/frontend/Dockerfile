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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.4@sha256:3444149d0a7e3f7cfb9c2db65f0f75676fe6ad04de3ce72674efb120c08dd1c1 AS buildbase

# Compile the UI assets.
FROM --platform=$BUILDPLATFORM us-central1-docker.pkg.dev/serverless-runtimes/google-24-full/runtimes/nodejs24@sha256:b84e743ae17a3ee6de75c43e368790195ae4e98585f227f350a00e8ad393edca AS assets
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
ARG TARGETOS
ARG TARGETARCH
ARG BUILDARCH
WORKDIR /app
COPY charts/values.global.yaml charts/values.global.yaml
COPY --from=assets /app ./

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
	-tags builtinassets \
	-ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | go tool -modfile="tools/go.mod" yq ".version" ) \
	-X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o frontend \
	cmd/frontend/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20260615.00_p0@sha256:57d7404dd1d7246cef57784b3532ee23d31ee8a13083f51bf37c15499e40b754
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
