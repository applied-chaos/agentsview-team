alter table users add column last_sign_in_at timestamptz;

create table pending_admins (
    email text primary key,
    created_at timestamptz not null
);

-- Names are copied in, not referenced, so entries outlive deleted Users.
create table audit_log (
    id bigserial primary key,
    at timestamptz not null,
    actor_id uuid,
    actor_name text not null,
    action text not null,
    category text not null,
    target_id uuid,
    target_label text not null,
    details text not null default ''
);

create index audit_log_at_idx on audit_log (at desc, id desc);
