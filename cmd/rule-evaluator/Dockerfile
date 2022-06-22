FROM --platform=$BUILDPLATFORM golang:1.17-buster AS buildbase
WORKDIR /app
COPY . ./

FROM --platform=$BUILDPLATFORM buildbase as appbase
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -mod=vendor -o rule-evaluator cmd/rule-evaluator/*.go

FROM --platform=$BUILDPLATFORM gcr.io/distroless/static:latest
COPY --from=appbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
