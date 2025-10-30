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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.24.9@sha256:17706709dd6f55af1efb33509fea7f09f54163a1209871224bed4cf297fb6bbb AS buildbase
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor vendor
COPY cmd cmd
COPY pkg pkg
COPY internal internal

RUN if [ "${TARGETARCH}" = "arm64" ] && [ "${BUILDARCH}" != "arm64" ]; then \
      apt install -y --no-install-recommends \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross; \
      CC=aarch64-linux-gnu-gcc; \
    fi && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=1 CC=${CC} \
      go build -tags boring -mod=vendor -o rule-evaluator cmd/rule-evaluator/*.go

FROM gcr.io/distroless/base-nossl-debian12:nonroot
COPY --from=buildbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
