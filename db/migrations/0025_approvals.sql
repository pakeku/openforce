alter table automation_executions drop constraint automation_execution_status_ok;

alter table automation_executions
    add constraint automation_execution_status_ok
    check (status in ('skipped', 'done', 'waiting', 'rejected'));

create table approvals (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    automation_id uuid not null references automation_definitions (id) on delete cascade,
    event_id uuid not null references events (id),
    user_id uuid not null references users (id),
    created_at timestamptz not null default now(),
    unique (automation_id, event_id)
);

create index approvals_user_created on approvals (organization_id, user_id, created_at desc, id desc);
