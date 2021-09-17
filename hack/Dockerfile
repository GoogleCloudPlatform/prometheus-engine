FROM golang:1.15-buster as buildbase
FROM google/cloud-sdk:slim as kindtest

WORKDIR /build

# Install go.
COPY --from=buildbase /usr/local/go /usr/local

# Install docker CLI.
RUN curl -fsSL https://get.docker.com -o get-docker.sh
RUN sh get-docker.sh && rm get-docker.sh

# Install kubectl.
RUN curl -Lo ./kubectl https://dl.k8s.io/release/v1.22.0/bin/linux/amd64/kubectl
RUN install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl \
  && rm kubectl

# Install kind.
RUN curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.11.1/kind-linux-amd64
RUN install -o root -g root -m 0755 kind /usr/local/bin/kind \
  && rm kind

# Get resources into image.
COPY . .