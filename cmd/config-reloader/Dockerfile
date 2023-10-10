FROM golang:1.21-bullseye AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 go build -mod=vendor -o config-reloader cmd/config-reloader/*.go

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/config-reloader /bin/config-reloader
ENTRYPOINT ["/bin/config-reloader"]
