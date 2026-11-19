#!/bin/bash
# Provision one Instance: database, owner and runtime roles, hosted schema, grants, config.
# Usage: provision.sh <instance>. Re-runnable. Connects only as the non-superuser `platform` role.
set -euo pipefail
name=$1
data=/data/$name
pg_host=${PGHOST:-postgres}
mkdir -p "$data"

rand() { head -c "$1" /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n'; }
if [ ! -f "$data/secrets.env" ]; then
  umask 077
  cat >"$data/secrets.env" <<EOF
OWNER_PW=$(rand 24)
RT_PW=$(rand 24)
UI_TOKEN=$(rand 24)
CURSOR_SECRET=$(head -c 32 /dev/urandom | base64)
EOF
fi
# shellcheck disable=SC1091
. "$data/secrets.env"

db=$name owner=${name}_owner rt=${name}_rt tenant=$name schema=agentsview
admin="postgresql://platform:platform@$pg_host/postgres"

psql "$admin" -v ON_ERROR_STOP=1 -q \
  -v db="$db" -v owner="$owner" -v rt="$rt" -v owner_pw="$OWNER_PW" -v rt_pw="$RT_PW" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN', :'owner') WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'owner') \gexec
SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION NOINHERIT', :'rt') WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'rt') \gexec
SELECT format('ALTER ROLE %I PASSWORD %L', :'owner', :'owner_pw') \gexec
SELECT format('ALTER ROLE %I PASSWORD %L', :'rt', :'rt_pw') \gexec
-- PG16+: CREATE DATABASE ... OWNER needs SET on the owner; the creator only gets ADMIN.
SELECT format('GRANT %I TO platform WITH INHERIT FALSE, SET TRUE', :'owner') \gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'db', :'owner') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'db') \gexec
SQL

owner_url="postgresql://$owner:$OWNER_PW@$pg_host/$db?sslmode=disable"
rt_url="postgresql://$rt:$RT_PW@$pg_host/$db?sslmode=disable"

umask 077
cat >"$data/config.toml" <<EOF
default_pg = "hosted"
require_auth = true
auth_token = "$UI_TOKEN"
cursor_secret = "$CURSOR_SECRET"

[pg.provision]
url = "$owner_url"
schema = "$schema"
raw_tenant = "$tenant"
allow_insecure = true

[pg.hosted]
url = "$rt_url"
schema = "$schema"
raw_tenant = "$tenant"
raw_derivation = true
raw_poll_seconds = 2
allow_insecure = true
EOF

psql "$owner_url" -v ON_ERROR_STOP=1 -q <<SQL
REVOKE ALL ON DATABASE $db FROM PUBLIC;
GRANT CONNECT ON DATABASE $db TO $rt;
CREATE SCHEMA IF NOT EXISTS $schema;
SQL
AGENTSVIEW_DATA_DIR=$data agentsview replica pg hosted-provision provision

# Runtime grants from upstream docs/hosted-raw-sync.md (v0.47.0).
psql "$owner_url" -v ON_ERROR_STOP=1 -q <<SQL
SET search_path = $schema;
GRANT USAGE ON SCHEMA $schema TO $rt;
GRANT SELECT ON ALL TABLES IN SCHEMA $schema TO $rt;
GRANT UPDATE (dialogue_text) ON messages TO $rt;
GRANT INSERT ON raw_device_tokens, raw_manifest_entries, raw_manifest_objects, messages, tool_calls,
  tool_result_events, usage_events, session_plan_usage, secret_findings, excluded_sessions,
  raw_projection_generations, raw_source_contributions, raw_session_public_aliases,
  raw_embedding_outbox TO $rt;
GRANT UPDATE (observed_outcome, observed_repeat, sequence_ending) ON tool_calls TO $rt;
GRANT INSERT, UPDATE ON raw_objects, raw_source_heads, raw_ingest_jobs, raw_source_projections,
  raw_session_groups, raw_content_revisions, raw_session_branches, raw_corpus_state,
  raw_projection_rollouts TO $rt;
GRANT INSERT ON raw_manifests TO $rt;
GRANT SELECT, INSERT, UPDATE, DELETE ON raw_message_dialogue TO $rt;
GRANT INSERT, UPDATE, DELETE ON raw_upload_sessions, sessions, session_sources, pinned_messages,
  raw_curation, raw_pins TO $rt;
GRANT INSERT, DELETE ON starred_sessions, raw_session_links, friction_findings TO $rt;
GRANT USAGE ON SEQUENCE raw_ingest_jobs_id_seq, tool_calls_id_seq, tool_result_events_id_seq,
  usage_events_id_seq, pinned_messages_id_seq TO $rt;
-- Upstream's docs omit some of these; this list is hostedProjectionPrivileges in
-- internal/postgres/raw_runtime_privileges.go, which runtime start-up checks.
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions, pinned_messages, raw_message_dialogue, session_sources,
  raw_curation, raw_pins TO $rt;
GRANT SELECT, INSERT ON messages, tool_calls, tool_result_events, usage_events, session_plan_usage,
  secret_findings, raw_projection_generations, raw_source_contributions, raw_session_public_aliases,
  raw_embedding_outbox TO $rt;
GRANT SELECT, INSERT, DELETE ON friction_findings, starred_sessions, raw_session_links TO $rt;
GRANT SELECT, INSERT, UPDATE ON excluded_sessions, raw_source_projections, raw_session_groups,
  raw_content_revisions, raw_session_branches, raw_corpus_state, raw_projection_rollouts TO $rt;
SQL
echo "provisioned $name"
