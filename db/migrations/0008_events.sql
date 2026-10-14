create table events (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    seq bigint generated always as identity,
    name text not null,
    actor_id uuid not null references users (id),
    object_id uuid references object_definitions (id),
    record_id uuid references records (id),
    source text not null,
    payload jsonb not null default '{}',
    created_at timestamptz not null default now(),
    constraint events_name_ok check (name in (
        'record.created',
        'record.updated',
        'record.deleted',
        'field.changed',
        'user.created',
        'object.created',
        'workflow.started',
        'workflow.completed'
    )),
    constraint events_source_ok check (source in (
        'UI',
        'API',
        'Automation',
        'Integration',
        'Import',
        'Admin'
    ))
);

create index events_org_seq on events (organization_id, seq);
