
FROM golang:1.15-buster AS buildbase

# Compile the UI assets.
FROM launcher.gcr.io/google/nodejs as assets
# To build the UI we need a recent node version and the go toolchain.
RUN install_node v14.16.1
COPY --from=buildbase /usr/local/go /usr/local/
ENV PATH="/usr/local/go/bin:${PATH}"
COPY . /app
RUN /app/pkg/ui/build.sh

# Build the actual Go binary.
FROM buildbase AS appbase
WORKDIR /app
COPY --from=assets /app ./
RUN CGO_ENABLED=0 go build -tags builtinassets -mod=vendor -o frontend ./cmd/frontend/*.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
