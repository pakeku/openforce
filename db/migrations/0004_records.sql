create table records (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    object_id uuid not null references object_definitions (id),
    data jsonb not null default '{}',
    version int not null default 1,
    deleted_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index records_org_object_live
    on records (organization_id, object_id)
    where deleted_at is null;
