ARG AGENTSVIEW_VERSION=0.47.0
FROM ghcr.io/kenn-io/agentsview:${AGENTSVIEW_VERSION} AS agentsview

FROM debian:bookworm-slim AS base
ARG AGENTSVIEW_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates procps util-linux tar gzip \
    && rm -rf /var/lib/apt/lists/*
COPY --from=agentsview /usr/local/bin/agentsview /tmp/agentsview
RUN arch=$(dpkg --print-architecture) && dir=/srv/release/v${AGENTSVIEW_VERSION} && mkdir -p "$dir" \
    && tar -czf "$dir/agentsview_${AGENTSVIEW_VERSION}_linux_${arch}.tar.gz" -C /tmp agentsview \
    && (cd "$dir" && sha256sum ./*.tar.gz | sed 's# \./# #' > SHA256SUMS) && rm /tmp/agentsview
# The platform service shares this network namespace, so it is loopback (the raw-sync client allows plain HTTP there only).
ENV AV_DOWNLOAD_BASE=file:///srv/release AV_PLATFORM_URL=http://127.0.0.1:8080

FROM base AS profile
CMD ["sleep", "infinity"]

FROM base AS systemd
RUN apt-get update && apt-get install -y --no-install-recommends systemd systemd-sysv dbus dbus-user-session \
    && rm -rf /var/lib/apt/lists/*
STOPSIGNAL SIGRTMIN+3
CMD ["/lib/systemd/systemd"]
