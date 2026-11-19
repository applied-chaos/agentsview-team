#!/bin/bash
# Wait until Instance <name> has no pending parse jobs, then print job counts by state.
set -euo pipefail
name=$1
# shellcheck disable=SC1090
. "/data/$name/secrets.env"
q() {
  psql -AtqX "postgresql://${name}_owner:$OWNER_PW@${PGHOST:-postgres}/$name" \
    -c "SET agentsview.tenant_id TO '$name'" -c "$1"
}
while [ "$(q "SELECT count(*) FROM agentsview.raw_ingest_jobs WHERE state IN ('ready', 'leased', 'retrying')")" != 0 ]; do
  sleep 2
done
q "SELECT state || '=' || count(*) FROM agentsview.raw_ingest_jobs GROUP BY state ORDER BY 1"
