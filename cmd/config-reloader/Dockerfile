FROM golang:1.17-buster AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 go build -mod=vendor -o config-reloader cmd/config-reloader/*.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/config-reloader /bin/config-reloader
ENTRYPOINT ["/bin/config-reloader"]
