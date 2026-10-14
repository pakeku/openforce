create table outbound_emails (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    to_address text not null,
    subject text not null,
    body text not null,
    event_id uuid not null references events (id),
    created_at timestamptz not null default now()
);

create index outbound_emails_org_created on outbound_emails (organization_id, created_at desc, id desc);
