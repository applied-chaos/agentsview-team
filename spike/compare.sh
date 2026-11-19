#!/bin/bash
# Print a device-independent fingerprint of every session in Instance <name>, sorted.
# Diff two Instances' output to compare them. Usage: compare.sh <name>
set -euo pipefail
name=$1
# shellcheck disable=SC1090
. "/data/$name/secrets.env"
psql -AtqX "postgresql://${name}_owner:$OWNER_PW@${PGHOST:-postgres}/$name" -v tenant="$name" <<'SQL'
SET agentsview.tenant_id TO :'tenant';
SET search_path = agentsview;
SELECT s.source_session_id, s.project, s.agent, s.cwd, s.git_branch, s.started_at, s.ended_at,
  s.message_count, s.user_message_count, s.total_output_tokens, s.peak_context_tokens,
  s.health_score, s.outcome, s.friction_count, md5(coalesce(s.first_message, '')),
  (SELECT md5(string_agg(m.role || ':' || coalesce(m.content, ''), '|' ORDER BY m.ordinal)) FROM messages m WHERE m.session_id = s.id),
  (SELECT count(*) FROM tool_calls t WHERE t.session_id = s.id),
  (SELECT coalesce(sum(u.input_tokens + u.output_tokens + u.cache_read_input_tokens + u.cache_creation_input_tokens), 0)
     FROM usage_events u WHERE u.session_id = s.id)
FROM sessions s WHERE s.deleted_at IS NULL ORDER BY 1;
SQL
