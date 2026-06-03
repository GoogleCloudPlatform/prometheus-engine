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
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY charts/values.global.yaml charts/values.global.yaml
COPY go.mod go.mod
COPY go.sum go.sum
# Copy the Go vendor directory only if it exists. Vendor folder will automatically
# cause 'go build' to use -mod=vendor flag (otherwise -mod=mod is used).
COPY vendor* vendor
COPY cmd cmd
COPY pkg pkg
COPY internal internal

ENV GOEXPERIMENT=noboringcrypto
ENV CGO_ENABLED=0
ENV GOFIPS140=certified
ENV GOTOOLCHAIN=local
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
RUN go build \
	-ldflags="-X github.com/prometheus/common/version.Version=$(cat charts/values.global.yaml | go tool yq '.version' ) \
	-X github.com/prometheus/common/version.BuildDate=$(date --iso-8601=seconds)" \
	-o rule-evaluator \
	cmd/rule-evaluator/*.go


FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
COPY --from=buildbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
