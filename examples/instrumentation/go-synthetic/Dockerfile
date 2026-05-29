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
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY . ./

FROM buildbase AS appbase

ENV GOEXPERIMENT=noboringcrypto
ENV CGO_ENABLED=0
ENV GOFIPS140=certified
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
RUN go build -o go-synthetic ./examples/instrumentation/go-synthetic

FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
COPY --from=appbase /app/go-synthetic /bin/go-synthetic
ENTRYPOINT ["/bin/go-synthetic"]
