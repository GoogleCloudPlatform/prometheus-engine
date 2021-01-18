# TODO(freinartz): pick a proper base image and figure out how to build the reloader for it
# The standard busybox image appears to have linking issues with naively build Go executables.
FROM golang:latest
LABEL maintainer="The Google Prometheus Engine Authors"

COPY ./config-reloader /reloader

ENTRYPOINT [ "/reloader" ]