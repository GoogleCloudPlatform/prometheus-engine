FROM golang:1.20 AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase

RUN CGO_ENABLED=0 go build -o example-app ./cmd/example-app/main.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/example-app /bin/example-app
ENTRYPOINT ["/bin/example-app"]