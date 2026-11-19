FROM golang:1.27.3-bookworm AS replay
RUN git clone -q --depth 1 --branch v0.47.0 https://github.com/kenn-io/agentsview.git /src
WORKDIR /src
# The replay tool uses upstream's internal raw-sync client, so it must build inside the module.
COPY replay/main.go cmd/spike-replay/main.go
# As upstream's Dockerfile: sqlite-vec's cgo bindings need the bundled sqlite3 header.
RUN go mod download && mkdir -p /sqlite-include \
    && cp "$(go list -m -f '{{.Dir}}' github.com/mattn/go-sqlite3)/sqlite3-binding.h" /sqlite-include/sqlite3.h
ENV CGO_CFLAGS="-O2 -g -I/sqlite-include"
RUN go run ./internal/pricing/cmd/litellm-snapshot -restore && CGO_ENABLED=1 go build -tags fts5 -trimpath -o /out/spike-replay ./cmd/spike-replay

FROM ghcr.io/kenn-io/agentsview:0.47.0
RUN apt-get update \
    && apt-get install -y --no-install-recommends postgresql-client python3 procps \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 10001 -m platform && chown platform /data
COPY --from=replay /out/spike-replay /usr/local/bin/spike-replay
COPY provision.sh enrol.sh platform.sh serve.sh replay.sh compare.sh wait-parsed.sh gensessions.py sample.py /spike/
ENTRYPOINT []
