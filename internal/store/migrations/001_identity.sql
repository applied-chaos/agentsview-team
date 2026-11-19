create table users (
    id uuid primary key default gen_random_uuid(),
    email text not null unique,
    name text not null,
    is_admin boolean not null default false,
    disabled boolean not null default false,
    created_at timestamptz not null default now()
);

create table refresh_tokens (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    family_id uuid not null,
    token_hash bytea not null unique,
    expires_at timestamptz not null,
    used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz not null
);

create index refresh_tokens_family_idx on refresh_tokens (family_id);
create index refresh_tokens_user_idx on refresh_tokens (user_id);
