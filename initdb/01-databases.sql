-- The only superuser step: the Platform's own role, Aurora-like (CREATEDB and CREATEROLE, no SUPERUSER or BYPASSRLS).
create role platform login password 'platform' createdb createrole nosuperuser nobypassrls noreplication;
create database platform owner platform;
create database platform_prod owner platform;
-- Instance databases must not be reachable through PUBLIC.
revoke connect on database postgres from public;
grant connect on database postgres to platform;
