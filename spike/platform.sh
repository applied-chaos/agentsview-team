#!/bin/bash
# Stand-in for the Platform supervisor: provision then run each Instance as a child process.
# INSTANCES="a b c" gives a on :8081, b on :8082, ... Logs in /data/<instance>/serve.log.
set -euo pipefail
port=8081
mkdir -p "${TMPDIR:-/tmp}"
for name in ${INSTANCES:?}; do
  /spike/provision.sh "$name" >"/data/$name.provision.log" 2>&1 || { cat "/data/$name.provision.log"; exit 1; }
  echo "$port" >"/data/$name/port"
  /spike/serve.sh "$name" &
  echo "$name pid=$! port=$port"
  port=$((port + 1))
done
wait
sleep infinity
