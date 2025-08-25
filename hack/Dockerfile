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

# deps builds binaries in an isolated environment to avoid
# funkiness in the hermetic build.
FROM google-go.pkg.dev/golang:1.25.0@sha256:ffc9c87d9826ce5c48de165fb774e3ec0c4fb1006aa09b374b6cf2e1638e5b89 AS deps
WORKDIR /workspace

RUN go install github.com/bwplotka/bingo@v0.9.0
COPY .bingo .bingo
RUN bingo -v get # TODO(bwplotka): It takes 177s in docker container with colima on Mac for me. Investigate or de-dockerize.

# hermetic is a lite copy of the repo resources used in building
# testing in a hermetic, idempotent, and reproducable environment.
FROM google-go.pkg.dev/golang:1.25.0@sha256:ffc9c87d9826ce5c48de165fb774e3ec0c4fb1006aa09b374b6cf2e1638e5b89 AS hermetic

COPY --from=deps /go/bin /go/bin
ARG RUNCMD='go fmt ./...'
WORKDIR /workspace

# Separate COPY directives to take advantage of docker's build cache.
# Least-changed dirs should go first. NOTE: When adding new directories
# ensure .dockerignore is updated.
COPY *.md *.md
COPY vendor* vendor
COPY hack hack
COPY .bingo .bingo
COPY go.mod go.mod
COPY go.sum go.sum
COPY examples examples
COPY manifests manifests
COPY cmd cmd
COPY charts charts
COPY doc doc
COPY pkg pkg
COPY e2e e2e
COPY internal internal

# Init a dummy git repo so we can check if generated code changes via
# git diff.
RUN git config --global user.email "test@test.com" \
	&& git init && git add . && git commit -am 'base'
# Hack to get multiline build arg to run properly.
RUN echo ${RUNCMD} | sh && echo 'done'

# sync is used to copy all auto-generated files to a different context.
# Usually this is used to mirror the changes back to the host machine.
FROM scratch AS sync
COPY --from=hermetic /workspace/go.mod go.mod
COPY --from=hermetic /workspace/go.sum go.sum
COPY --from=hermetic /workspace/cmd cmd
COPY --from=hermetic /workspace/doc doc
COPY --from=hermetic /workspace/examples examples
COPY --from=hermetic /workspace/manifests manifests
COPY --from=hermetic /workspace/charts charts
COPY --from=hermetic /workspace/e2e e2e

## kindtest image for running tests against kind cluster in hermetic environment.
FROM google-go.pkg.dev/golang:1.25.0@sha256:ffc9c87d9826ce5c48de165fb774e3ec0c4fb1006aa09b374b6cf2e1638e5b89 AS buildbase
FROM docker:28.3-cli AS docker
FROM debian:stable-slim AS kindtest

WORKDIR /build

# Install go.
COPY --from=buildbase /usr/local/go /usr/local

# Install docker cli.
COPY --from=docker /usr/local/bin/docker /usr/local/bin

# Install curl.
RUN apt update
RUN apt install -y curl

# Install kubectl, jq.
RUN curl -Lo ./kubectl https://dl.k8s.io/release/v1.32.1/bin/linux/amd64/kubectl
RUN install -o root -g root -m 07555 kubectl /usr/local/bin/kubectl \
  && rm kubectl
RUN curl -Lo ./jq https://github.com/jqlang/jq/releases/download/jq-1.7/jq-linux-amd64
RUN install -o root -g root -m 07555 jq /usr/local/bin/jq \
  && rm jq

# Install kind.
RUN curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.26.0/kind-linux-amd64
RUN install -o root -g root -m 0755 kind /usr/local/bin/kind \
  && rm kind

# Get resources into image.
COPY examples examples
COPY cmd cmd
COPY manifests manifests
COPY charts charts
COPY vendor* vendor
COPY pkg pkg
COPY e2e e2e
COPY hack hack
COPY go.mod go.mod
COPY go.sum go.sum
