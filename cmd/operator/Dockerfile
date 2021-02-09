ARG APP=operator

FROM golang:1.15-buster AS builder
ARG APP
WORKDIR /app
COPY . ./
RUN CGO_ENABLED=0 go build -mod=vendor -o ${APP} cmd/${APP}/*.go

FROM gcr.io/distroless/static:latest
ARG APP
COPY --from=builder /app/${APP} /bin/${APP}
ENTRYPOINT ["/bin/operator"]