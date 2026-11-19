-- The Admin Instance is one more tenant; its id is random per Platform database so two
-- Platforms sharing a Postgres server never share an Instance database.
create table admin_instance (
    singleton boolean primary key default true check (singleton),
    id uuid not null,
    db_name text not null unique,
    owner_password bytea not null,
    runtime_password bytea not null,
    bearer_token bytea not null,
    cursor_secret bytea not null,
    created_at timestamptz not null default now()
);

-- Spool of manifests the Instance has committed, in commit order, awaiting replay.
create table replay_manifests (
    id bigint generated always as identity primary key,
    user_id uuid not null references users (id) on delete cascade,
    device_id text not null,
    manifest_id text not null,
    body bytea not null,
    received_at timestamptz not null,
    replayed_at timestamptz,
    unique (user_id, manifest_id)
);

create index replay_manifests_pending_idx on replay_manifests (id) where replayed_at is null;

-- One mirrored device in the Admin Instance per Host; the Credential is sealed.
create table replay_devices (
    host_id uuid primary key references hosts (id) on delete cascade,
    device_id text not null unique,
    credential bytea not null,
    name text not null default ''
);

-- The Admin Instance's own receipt chain per source, so replay resumes without asking it.
create table replay_heads (
    device_id text not null,
    provider text not null,
    root_id text not null,
    source_hash bytea not null,
    receipt text not null,
    primary key (device_id, provider, root_id, source_hash)
);
