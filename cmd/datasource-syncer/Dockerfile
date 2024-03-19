FROM golang:1.21-bullseye AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 GOEXPERIMENT=boringcrypto go build -mod=vendor -o datasource-syncer cmd/datasource-syncer/*.go

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/datasource-syncer /bin/datasource-syncer
ENTRYPOINT ["/bin/datasource-syncer"]

