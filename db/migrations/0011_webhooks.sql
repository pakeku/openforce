create table webhook_subscriptions (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    url text not null,
    secret text not null,
    events jsonb not null,
    created_at timestamptz not null default now(),
    unique (organization_id, url)
);

alter table jobs add column subscription_id uuid references webhook_subscriptions (id) on delete cascade;
alter table jobs drop constraint jobs_event_id_kind_key;
alter table jobs add constraint jobs_event_kind_target unique nulls not distinct (event_id, kind, subscription_id);
