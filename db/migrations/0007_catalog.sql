create table view_definitions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    object_id uuid not null references object_definitions (id),
    name text not null,
    label text not null,
    kind text not null default 'table',
    columns jsonb not null,
    filters jsonb not null default '[]',
    sort jsonb not null default '[]',
    version int not null default 1,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (organization_id, object_id, name),
    constraint view_kind_ok check (kind in ('table'))
);

create table layout_definitions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    object_id uuid not null references object_definitions (id),
    name text not null,
    label text not null,
    fields jsonb not null,
    version int not null default 1,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (organization_id, object_id, name)
);

create table page_definitions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    object_id uuid not null references object_definitions (id),
    name text not null,
    label text not null,
    document jsonb not null,
    version int not null default 1,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (organization_id, object_id, name)
);
