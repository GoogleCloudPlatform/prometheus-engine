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

FROM --platform=$BUILDPLATFORM google-go.pkg.dev/golang:1.26.7@sha256:2f110d472e26e5c6b1c93f40a3ea6f88fe8954902cd4518d7ce99d836a158ceb AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase AS appbase
ARG TARGETOS
ARG TARGETARCH
ARG BUILDARCH

ENV GOEXPERIMENT=boringcrypto
ENV CGO_ENABLED=1
ENV GOFIPS140=off
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
	go build -o go-synthetic ./examples/instrumentation/go-synthetic

FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20260815.00_p0@sha256:532c7001d381c6f0c6fe684bb3235596cd532340c463df855a0ea168a83d783e
COPY --from=appbase /app/go-synthetic /bin/go-synthetic
ENTRYPOINT ["/bin/go-synthetic"]
