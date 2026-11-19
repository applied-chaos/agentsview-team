alter table install_tokens
    add column description text not null default '',
    add column reusable boolean not null default false,
    add column ephemeral boolean not null default false,
    add column last_used_at timestamptz,
    add column revoked_at timestamptz;

alter table hosts
    add column ephemeral boolean not null default false,
    add column install_token_id uuid references install_tokens (id) on delete set null;

update hosts h set install_token_id = t.id from install_tokens t where t.host_id = h.id;
