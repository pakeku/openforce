create table functions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    name text not null,
    label text not null,
    created_at timestamptz not null default now(),
    unique (organization_id, name)
);
