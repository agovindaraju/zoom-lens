# syntax=docker/dockerfile:1
# OpAMP Supervisor + OpenTelemetry Collector Contrib in one image.
#
# The official Supervisor image ships only the Supervisor binary
# (/usr/local/bin/opampsupervisor), so the contrib collector binary is copied
# in from the official contrib image. Both use the same pinned version.
#
#   docker build -f build/collector.Dockerfile -t zoomlens-collector:dev .

ARG OTEL_VERSION=0.161.0

FROM otel/opentelemetry-collector-contrib:${OTEL_VERSION} AS collector

FROM ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-opampsupervisor:${OTEL_VERSION}
COPY --from=collector --chmod=755 /otelcol-contrib /otelcol-contrib
ENTRYPOINT ["opampsupervisor"]
CMD ["--config", "/etc/zoomlens/supervisor.yaml"]
