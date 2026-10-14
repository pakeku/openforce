create table audit_entries (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    event_id uuid not null unique references events (id),
    actor_id uuid not null references users (id),
    object_id uuid not null references object_definitions (id),
    record_id uuid not null references records (id),
    name text not null,
    field text,
    before jsonb,
    after jsonb,
    source text not null,
    created_at timestamptz not null,
    constraint audit_name_ok check (name in ('field.changed', 'record.deleted')),
    constraint audit_source_ok check (source in (
        'UI',
        'API',
        'Automation',
        'Integration',
        'Import',
        'Admin'
    ))
);

create index audit_org_record on audit_entries (organization_id, record_id);
