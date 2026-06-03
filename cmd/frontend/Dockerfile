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
FROM --platform=$BUILDPLATFORM us-central1-docker.pkg.dev/serverless-runtimes/google-24-full/runtimes/nodejs24@sha256:113cf6a49bb51dfb63dd2e7dbeb51bf183a467e6ed2c0855e656d78ed2eabc15 AS assets
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
COPY charts/values.global.yaml charts/values.global.yaml
COPY --from=assets /app ./

ENV GOEXPERIMENT=noboringcrypto
ENV CGO_ENABLED=0
ENV GOFIPS140=certified
ENV GOTOOLCHAIN=local
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
RUN go build \
	-tags builtinassets \
	-ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | go tool yq ".version" ) \
	-X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o frontend \
	cmd/frontend/*.go


FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
