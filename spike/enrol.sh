#!/bin/bash
# Enrol a raw-sync device in an Instance by writing upstream's raw_devices row (no upstream endpoint).
# Usage: enrol.sh <instance> <display name>. Writes /data/<instance>/devices/<display name>.env once.
set -euo pipefail
name=$1 display=$2
data=/data/$name
out="$data/devices/${display//\//_}.env"
[ -f "$out" ] && { cat "$out"; exit 0; }
# shellcheck disable=SC1091
. "$data/secrets.env"
rand() { head -c "$1" /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n'; }
device_id=dev_$(rand 16)
credential=avdc_$(rand 32)
# Tables use FORCE ROW LEVEL SECURITY, so even the owner must set the tenant.
psql "postgresql://${name}_owner:$OWNER_PW@${PGHOST:-postgres}/$name" -v ON_ERROR_STOP=1 -q \
  -v tenant="$name" -v dev="$device_id" -v disp="$display" -v cred="$credential" <<'SQL'
SET agentsview.tenant_id TO :'tenant';
INSERT INTO agentsview.raw_devices (device_id, tenant_id, display_name, credential_sha256, created_at)
VALUES (:'dev', :'tenant', :'disp', sha256(convert_to(:'cred', 'UTF8')), now());
SQL
mkdir -p "$data/devices"
umask 077
printf 'AGENTSVIEW_RAW_SYNC_DEVICE_ID=%s\nAGENTSVIEW_RAW_SYNC_CREDENTIAL=%s\n' "$device_id" "$credential" >"$out"
cat "$out"
