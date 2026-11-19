#!/bin/bash
# Run one provisioned Instance in the foreground on the port platform.sh gave it.
set -euo pipefail
name=$1
port=$(cat "/data/$name/port")
exec env AGENTSVIEW_DATA_DIR="/data/$name" AGENTSVIEW_TELEMETRY_ENABLED=0 \
  agentsview replica pg serve --host 0.0.0.0 --port "$port" --no-browser \
  --public-url "http://localhost:$port" >>"/data/$name/serve.log" 2>&1
