# The one place the agentsview release is pinned: the base image and, via -ldflags, the Platform.
ARG AGENTSVIEW_VERSION=0.47.0

# Web and Go stages run natively on the build host; only the final stage is per-platform.
FROM --platform=$BUILDPLATFORM node:22-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS gobuild
ARG GO_TAGS=""
ARG AGENTSVIEW_VERSION
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -tags "$GO_TAGS" -trimpath -ldflags "-X github.com/applied-chaos/agentsview-team/internal/instance.PinnedVersion=${AGENTSVIEW_VERSION}" -o /out/platform ./cmd/platform \
    && CGO_ENABLED=0 GOARCH=$TARGETARCH go build -trimpath -o /out/fakeoidc ./cmd/fakeoidc

FROM golang:1.26-bookworm AS e2e
COPY --from=docker:27-cli /usr/local/bin/docker /usr/local/bin/docker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY --from=gobuild /out/platform /usr/local/bin/platform
COPY e2e/ e2e/
CMD ["go", "test", "-tags", "e2e", "-count=1", "-parallel", "8", "-v", "-timeout", "30m", "./e2e/..."]

FROM debian:bookworm-slim AS fakeoidc
COPY --from=gobuild /out/fakeoidc /usr/local/bin/fakeoidc
USER 10001
ENTRYPOINT ["/usr/local/bin/fakeoidc"]

# The pinned agentsview release is the base image, so Instances run its own binary (ADR 0003).
FROM ghcr.io/kenn-io/agentsview:${AGENTSVIEW_VERSION} AS platform
# The sandboxed parser needs a passwd home for vault locks and a writable /data/tmp.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && useradd -u 10001 -m platform && mkdir -p /data && chown platform /data
COPY --from=gobuild /out/platform /usr/local/bin/platform
COPY --from=web /web/dist /app/web
ENV PLATFORM_WEB_DIR=/app/web PLATFORM_DATA_DIR=/data
USER 10001
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=12 CMD ["/usr/local/bin/platform", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/platform"]
