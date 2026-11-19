-- The only superuser step: an Aurora-like master role without SUPERUSER, BYPASSRLS or REPLICATION.
CREATE ROLE platform LOGIN PASSWORD 'platform' CREATEDB CREATEROLE NOSUPERUSER NOBYPASSRLS NOREPLICATION;
-- Instance databases must not be reachable by every role through PUBLIC.
REVOKE CONNECT ON DATABASE postgres FROM PUBLIC;
GRANT CONNECT ON DATABASE postgres TO platform;
