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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.11@sha256:dcf06becb8142d3ac50c4615c3230f551a4e754e885287da5f844cf899e62228 AS buildbase

# Compile the UI assets.
FROM --platform=$BUILDPLATFORM us-central1-docker.pkg.dev/serverless-runtimes/google-24-full/runtimes/nodejs24@sha256:3d26abb4ab7c10b9aecd5dd901b7fc225f7d2f92effa94e7c485f2a12468208d AS assets
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
WORKDIR /app
COPY --from=assets /app ./

ENV GOEXPERIMENT=boringcrypto
ENV CGO_ENABLED=1
ENV GOFIPS140=off
ENV GOTOOLCHAIN=local
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
      apt install -y --no-install-recommends \
        -tags builtinassets \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
      CC=aarch64-linux-gnu-gcc; \
    fi && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CC=${CC} \
      go build \
        -tags builtinassets \
        -ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | go tool yq ".version" ) \
        -X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o frontend \
	cmd/frontend/*.go


FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20261101.00_p0@sha256:52e1fd9180da347fc8d735c50368872b33d7e9505b09a70a861ebe57940bf0c6
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
