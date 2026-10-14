create table automation_definitions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    name text not null,
    label text not null,
    event_name text not null,
    object_id uuid not null references object_definitions (id),
    conditions jsonb not null,
    actions jsonb not null,
    version int not null default 1,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (organization_id, name),
    constraint automation_event_ok check (event_name in (
        'record.created',
        'record.updated',
        'record.deleted'
    ))
);

create table automation_executions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    automation_id uuid not null references automation_definitions (id) on delete cascade,
    event_id uuid not null references events (id),
    status text not null,
    created_ids jsonb not null,
    created_at timestamptz not null default now(),
    unique (automation_id, event_id),
    constraint automation_execution_status_ok check (status in ('skipped', 'done'))
);

alter table jobs add column automation_id uuid references automation_definitions (id) on delete cascade;
alter table jobs drop constraint jobs_event_kind_target;
alter table jobs add constraint jobs_event_kind_target unique nulls not distinct (event_id, kind, subscription_id, automation_id);
