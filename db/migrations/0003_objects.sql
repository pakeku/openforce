create table object_definitions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    name text not null,
    label text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (organization_id, name)
);

create table field_definitions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    object_id uuid not null references object_definitions (id),
    name text not null,
    field_type text not null,
    required boolean not null default false,
    options jsonb,
    position int not null default 0,
    unique (object_id, name),
    constraint field_type_ok check (
        field_type in ('text', 'number', 'currency', 'date', 'boolean', 'enum')
    )
);
