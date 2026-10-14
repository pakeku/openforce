create table connector_syncs (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    name text not null,
    label text not null,
    credential_id uuid not null references credentials (id),
    object_id uuid not null references object_definitions (id),
    url text not null,
    external_id text not null,
    mapping jsonb not null,
    actor_id uuid not null references users (id),
    created_at timestamptz not null default now(),
    unique (organization_id, name)
);

create table connector_links (
    organization_id uuid not null references organizations (id),
    sync_id uuid not null references connector_syncs (id) on delete cascade,
    external_id text not null,
    record_id uuid not null references records (id),
    primary key (sync_id, external_id)
);

alter table events drop constraint events_name_ok;

alter table events add constraint events_name_ok check (name in (
    'record.created',
    'record.updated',
    'record.deleted',
    'field.changed',
    'user.created',
    'object.created',
    'workflow.started',
    'workflow.completed',
    'schedule.tick',
    'sync.requested'
));
