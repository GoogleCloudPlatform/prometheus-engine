FROM golang:1.15-buster AS buildbase
WORKDIR /app
COPY . ./
RUN go mod download

# Generate static assets during build.
FROM buildbase as assets
RUN apt-get update && apt-get install xz-utils
RUN curl https://nodejs.org/dist/v14.16.1/node-v14.16.1-linux-x64.tar.xz -o /tmp/node.tar.xz
RUN mkdir -p /opt/node && tar -xf /tmp/node.tar.xz -C /opt/node --strip-components 1
ENV PATH=/opt/node/bin:$PATH
RUN npm install -g yarn
RUN ./pkg/ui/sync.sh
RUN ./pkg/ui/build.sh

FROM assets as appbase
RUN CGO_ENABLED=0 go build -tags builtinassets -mod=mod -o frontend ./cmd/frontend/*.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/frontend /bin/frontend
ENTRYPOINT ["/bin/frontend"]
