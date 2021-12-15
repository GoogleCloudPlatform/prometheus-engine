FROM golang:1.17-buster AS buildbase
WORKDIR /app
COPY . ./

FROM buildbase as appbase
RUN CGO_ENABLED=0 go build -mod=vendor -o rule-evaluator cmd/rule-evaluator/*.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
