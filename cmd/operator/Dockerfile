FROM golang:1.21-bullseye AS buildbase
WORKDIR /app
COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd
COPY pkg pkg
COPY vendor vendor

FROM buildbase as appbase
RUN CGO_ENABLED=0 GOEXPERIMENT=boringcrypto go build -mod=vendor -o operator cmd/operator/*.go

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/operator /bin/operator
ENTRYPOINT ["/bin/operator"]
