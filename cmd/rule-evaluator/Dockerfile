FROM golang:1.15-buster AS buildbase
WORKDIR /app
COPY . ./
RUN go mod download

FROM buildbase as appbase
RUN CGO_ENABLED=0 go build -mod=mod -o rule-evaluator cmd/rule-evaluator/*.go

FROM gcr.io/distroless/static:latest
COPY --from=appbase /app/rule-evaluator /bin/rule-evaluator
ENTRYPOINT ["/bin/rule-evaluator"]
