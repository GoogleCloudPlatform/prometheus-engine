FROM golang:1.20-bookworm AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 go build -mod=vendor -o operator cmd/operator/*.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/operator /bin/operator
ENTRYPOINT ["/bin/operator"]
