#!/bin/sh
# Runs once, on an empty pgdata. The Platform's own role: CREATEDB and CREATEROLE, no SUPERUSER or BYPASSRLS.
set -eu
psql -v ON_ERROR_STOP=1 -v pw="$PLATFORM_DB_PASSWORD" -U postgres -d postgres <<'SQL'
create role platform login createdb createrole nosuperuser nobypassrls noreplication;
alter role platform password :'pw';
create database platform owner platform;
-- Instance databases must not be reachable through PUBLIC.
revoke connect on database postgres from public;
grant connect on database postgres to platform;
SQL
