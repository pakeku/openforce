create table object_permissions (
    organization_id uuid not null references organizations (id),
    role text not null,
    object_id uuid not null references object_definitions (id),
    can_read boolean not null,
    can_write boolean not null,
    primary key (organization_id, role, object_id)
);
