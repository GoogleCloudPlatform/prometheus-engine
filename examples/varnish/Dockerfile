FROM debian:stable-slim as stage
WORKDIR /exporter
ADD https://github.com/jonnenauha/prometheus_varnish_exporter/releases/download/1.6.1/prometheus_varnish_exporter-1.6.1.linux-amd64.tar.gz /exporter/exporter.tar.gz
RUN tar -xvf exporter.tar.gz
RUN chmod +x /exporter/prometheus_varnish_exporter-1.6.1.linux-amd64/prometheus_varnish_exporter
FROM varnish:7.2.0
COPY --from=stage /exporter/prometheus_varnish_exporter-1.6.1.linux-amd64/prometheus_varnish_exporter /prometheus_varnish_exporter
ENTRYPOINT [ "/prometheus_varnish_exporter" ]
