create table inbound_endpoints (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    name text not null,
    object_id uuid not null references object_definitions (id),
    secret text not null,
    actor_id uuid not null references users (id),
    created_at timestamptz not null default now(),
    unique (organization_id, name)
);
