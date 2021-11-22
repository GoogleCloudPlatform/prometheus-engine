# deps builds binaries in an isolated environment to avoid
# funkiness in the hermetic build.
FROM golang:1.17-buster as deps
WORKDIR /workspace
# Have to clone and install this as the go.mod uses replace directives.
RUN git clone --depth 1 --branch v0.50.0 https://github.com/prometheus-operator/prometheus-operator  \
  && cd prometheus-operator \
  && go install ./cmd/po-docgen
RUN go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0

# hermetic is a lite copy of the repo resources used in building
# testing in a hermetic, idempotent, and reproducable environment.
FROM golang:1.15-buster AS hermetic
COPY --from=deps /go/bin /go/bin
ARG RUNCMD='go fmt ./...'
WORKDIR /workspace
# Separate COPY directives to take advantage of docker's build cache.
# Least-changed dirs should go first.
COPY vendor vendor
COPY hack hack
COPY go.mod go.mod
COPY go.sum go.sum
COPY examples examples
COPY cmd cmd
COPY doc doc
COPY pkg pkg
# Init a dummy git repo so we can check if generated code changes via
# git diff.
RUN git config --global user.email "test@test.com" \
	&& git init && git add . && git commit -am 'base'
# Hack to get multiline build arg to run properly.
RUN echo ${RUNCMD} | sh && echo 'done'

# sync is used to copy all auto-generated files to a different context.
# Usually this is used to mirror the changes back to the host machine.
FROM scratch as sync
COPY --from=hermetic /workspace/go.mod go.mod
COPY --from=hermetic /workspace/go.sum go.sum
COPY --from=hermetic /workspace/cmd cmd
COPY --from=hermetic /workspace/doc doc
COPY --from=hermetic /workspace/examples examples
COPY --from=hermetic /workspace/hack hack
COPY --from=hermetic /workspace/pkg pkg
COPY --from=hermetic /workspace/vendor vendor