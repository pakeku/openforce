create table notifications (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    user_id uuid not null references users (id),
    title text not null,
    body text not null,
    event_id uuid not null references events (id),
    created_at timestamptz not null default now()
);

create index notifications_user_created on notifications (organization_id, user_id, created_at desc, id desc);
