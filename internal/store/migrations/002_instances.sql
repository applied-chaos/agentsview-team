-- Secrets are sealed (internal/secrets) before they reach this table.
create table instances (
    user_id uuid primary key references users (id) on delete cascade,
    db_name text not null unique,
    owner_password bytea not null,
    runtime_password bytea not null,
    bearer_token bytea not null,
    cursor_secret bytea not null,
    created_at timestamptz not null default now()
);
