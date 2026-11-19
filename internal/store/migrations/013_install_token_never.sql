-- A null expiry is an Install Token that never expires.
alter table install_tokens alter column expires_at drop not null;
