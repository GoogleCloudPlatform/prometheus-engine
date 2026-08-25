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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.6@sha256:fa5b46750a41635cb95da7f20fb21475f4ca25b82dbd51793ee22aec8db0cd0b AS buildbase

# Compile the UI assets.
FROM --platform=$BUILDPLATFORM us-central1-docker.pkg.dev/serverless-runtimes/google-24-full/runtimes/nodejs24@sha256:6743fa93d4a5ab1cd6b4061ff5947b84c997b2177db247852764d1c25412f12f AS assets
WORKDIR /app
# To build the UI we need a recent node version and the go toolchain.
COPY --from=buildbase /usr/bin/make /usr/bin/
COPY --from=buildbase /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY --chown=33:33 ./third_party/prometheus_ui ./third_party/prometheus_ui
COPY --chown=33:33 go.mod go.sum .
COPY --chown=33:33 ./cmd/frontend ./cmd/frontend
COPY --chown=33:33 ./internal/promapi ./internal/promapi
COPY --chown=33:33 ./pkg/ui ./pkg/ui
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
	-ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | go tool yq ".version" ) \
	-X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o frontend \
	cmd/frontend/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20260801.00_p0@sha256:0625a71ac0c78014343a6743ff368f50d1001769d4e181390dff83515deaf1f5
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
