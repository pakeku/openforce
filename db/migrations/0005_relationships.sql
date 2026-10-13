alter table field_definitions drop constraint field_type_ok;

alter table field_definitions add constraint field_type_ok check (
    field_type in ('text', 'number', 'currency', 'date', 'boolean', 'enum', 'relation')
);

create table relationship_definitions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    from_object_id uuid not null references object_definitions (id),
    to_object_id uuid not null references object_definitions (id),
    name text not null,
    kind text not null,
    inverse_name text,
    unique (from_object_id, name),
    constraint relationship_kind_ok check (kind in ('belongs_to', 'has_many', 'many_to_many'))
);

create table record_links (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    relationship_id uuid not null references relationship_definitions (id),
    from_record_id uuid not null references records (id),
    to_record_id uuid not null references records (id),
    unique (relationship_id, from_record_id, to_record_id)
);
