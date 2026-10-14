alter table memberships drop constraint membership_role_ok;

alter table memberships add constraint membership_role_ok check (role in ('owner', 'member', 'service'));

create table api_keys (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    user_id uuid not null unique references users (id),
    name text not null,
    label text not null,
    token_hash text not null unique,
    created_at timestamptz not null default now(),
    unique (organization_id, name)
);

create table api_key_permissions (
    api_key_id uuid not null references api_keys (id) on delete cascade,
    organization_id uuid not null references organizations (id),
    object_id uuid not null references object_definitions (id),
    can_read boolean not null,
    can_write boolean not null,
    primary key (api_key_id, object_id)
);
