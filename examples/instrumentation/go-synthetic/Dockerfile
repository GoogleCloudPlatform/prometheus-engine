FROM golang:1.20-bullseye AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase

RUN CGO_ENABLED=0 go build -o go-synthetic ./examples/instrumentation/go-synthetic

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/go-synthetic /bin/go-synthetic
ENTRYPOINT ["/bin/go-synthetic"]
