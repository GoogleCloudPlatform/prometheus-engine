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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.3@sha256:3e25fdade56f35be3bca41f8a141a66ca834f1c01fdf55414d4a1e1c987a985c AS buildbase

# Compile the UI assets.
FROM --platform=$BUILDPLATFORM gcr.io/google-appengine/nodejs AS assets
# To build the UI we need a recent node version and the go toolchain.
RUN install_node v17.9.0
COPY --from=buildbase /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY . /app
RUN pkg/ui/build.sh

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
