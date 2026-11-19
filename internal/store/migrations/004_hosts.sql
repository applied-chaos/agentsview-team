-- Install Tokens store only a hash.
create table install_tokens (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    token_hash bytea not null unique,
    expires_at timestamptz not null,
    used_at timestamptz,
    host_id uuid,
    created_at timestamptz not null
);

create index install_tokens_user_idx on install_tokens (user_id);

-- device_id is the raw-sync device in the User's Instance; its Credential is never stored here.
create table hosts (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    name text not null,
    device_id text not null unique,
    os text not null,
    install_mode text not null check (install_mode in ('systemd', 'profile')),
    client_version text not null,
    created_at timestamptz not null,
    last_seen_at timestamptz,
    last_upload_at timestamptz,
    revoked_at timestamptz
);

create unique index hosts_live_name_idx on hosts (user_id, lower(name)) where revoked_at is null;

alter table install_tokens add foreign key (host_id) references hosts (id) on delete set null;
