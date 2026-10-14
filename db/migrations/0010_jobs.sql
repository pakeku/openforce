create table jobs (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    event_id uuid not null references events (id),
    kind text not null,
    status text not null default 'pending',
    attempts int not null default 0,
    last_error text,
    run_at timestamptz not null default now(),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (event_id, kind),
    constraint jobs_status_ok check (status in ('pending', 'done', 'dead'))
);

create index jobs_due on jobs (status, run_at);
