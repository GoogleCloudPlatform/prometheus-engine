ARG APP=frontend

FROM golang:1.15-buster AS builder
ARG APP
WORKDIR /app
COPY . ./
# TODO(freinartz): generate static assets during build.
RUN CGO_ENABLED=0 go build -tags builtinassets -mod=vendor -o ${APP} cmd/${APP}/*.go

FROM gcr.io/distroless/static:latest
ARG APP
COPY --from=builder /app/${APP} /bin/${APP}
ENTRYPOINT ["/bin/frontend"]