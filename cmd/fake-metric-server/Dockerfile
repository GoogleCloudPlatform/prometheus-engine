FROM golang:1.17-buster AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 go build -mod=vendor -o fake-metric-server cmd/fake-metric-server/*.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/fake-metric-server /bin/fake-metric-server
EXPOSE 5678
ENTRYPOINT ["/bin/fake-metric-server"]