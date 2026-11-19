#!/bin/bash
# Replay every committed manifest of Instance <src> into Instance <dst> as device <display name>.
# Run inside the platform container. Stops <src> meanwhile: its raw-sync vault is exclusively locked.
# Usage: replay.sh <src> <dst> <display name>
set -euo pipefail
src=$1 dst=$2 display=$3
/spike/enrol.sh "$dst" "$display" >/dev/null
set -a
# shellcheck disable=SC1090,SC1091
. "/data/$src/secrets.env"
# shellcheck disable=SC1090
. "/data/$dst/devices/${display//\//_}.env"
set +a
port=$(cat "/data/$src/port")
pkill -f -- "--port $port " || true
while pgrep -f -- "--port $port " >/dev/null; do sleep 0.5; done
status=0
SRC_PG_URL="postgresql://${src}_owner:$OWNER_PW@${PGHOST:-postgres}/$src" SRC_TENANT=$src SRC_DATA_DIR=/data/$src \
  AGENTSVIEW_RAW_SYNC_URL="http://127.0.0.1:$(cat "/data/$dst/port")" spike-replay || status=$?
nohup /spike/serve.sh "$src" >/dev/null 2>&1 &
exit "$status"
