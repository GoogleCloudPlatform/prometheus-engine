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

# Compile the UI assets.
FROM --platform=$BUILDPLATFORM docker.io/node:26.6.0-bookworm@sha256:6e0a69f43d2b4a176fd4be503cdd041cae6d112ef27f03bd349303331cc19bd1 AS assets
WORKDIR /app
COPY --from=google-go.pkg.dev/golang:1.26.5@sha256:5075e8b0e1a7913c2274737c1a8b379ea264322c082bd21d3911d196dce46559 /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY . /app
RUN pkg/ui/build.sh

# sync is used to copy all auto-generated files to a different context.
# Usually this is used to mirror the changes back to the host machine.
FROM scratch AS sync
COPY --from=assets /app/pkg/ui/embed.go pkg/ui/embed.go
COPY --from=assets /app/pkg/ui/static pkg/ui/static

# Build the actual Go binary.
FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.6@sha256:fa5b46750a41635cb95da7f20fb21475f4ca25b82dbd51793ee22aec8db0cd0b AS buildbase
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY --from=assets /app ./
RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
      apt install -y --no-install-recommends \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
      CC=aarch64-linux-gnu-gcc; \
    fi && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=1 CC=${CC} \
      go build -tags boring,builtinassets -mod=vendor -o frontend cmd/frontend/*.go

FROM gcr.io/distroless/base-nossl-debian12:nonroot
COPY --from=buildbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
