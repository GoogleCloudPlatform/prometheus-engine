FROM golang:1.20-bullseye AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 GOEXPERIMENT=boringcrypto go build -mod=vendor -o rule-evaluator cmd/rule-evaluator/*.go

FROM gcr.io/distroless/static-debian11:latest
COPY --from=appbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
