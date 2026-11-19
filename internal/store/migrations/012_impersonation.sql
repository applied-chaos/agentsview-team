-- Impersonation is session state: it rides the refresh chain so it survives token refresh.
alter table refresh_tokens add column imp_user_id uuid, add column imp_started_at timestamptz;
