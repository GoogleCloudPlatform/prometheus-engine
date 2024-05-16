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

FROM google-go.pkg.dev/golang:1.22.3@sha256:efc6b824652199ede8bc25e2d5a57076e0e1ffbe90735dcbda3ee956fe33b612 as buildbase
WORKDIR /app
COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd
COPY pkg pkg
COPY vendor vendor

FROM buildbase as appbase
RUN CGO_ENABLED=1 GOEXPERIMENT=boringcrypto \
    go build -tags boring -mod=vendor -o config-reloader cmd/config-reloader/*.go

FROM gke.gcr.io/gke-distroless/libc:gke_distroless_20240307.00_p0@sha256:4f834e207f2721977094aeec4c9daee7032c5daec2083c0be97760f4306e4f88
COPY --from=appbase /app/config-reloader /bin/config-reloader
ENTRYPOINT ["/bin/config-reloader"]
