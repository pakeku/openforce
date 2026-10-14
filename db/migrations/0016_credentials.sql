create table credentials (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    name text not null,
    label text not null,
    kind text not null,
    secret text not null,
    token_url text,
    client_id text,
    client_secret text,
    refresh_token text,
    expires_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (organization_id, name),
    constraint credential_kind_ok check (kind in ('api_key', 'oauth')),
    constraint credential_shape_ok check (
        (
            kind = 'api_key'
            and secret <> ''
            and token_url is null
            and client_id is null
            and client_secret is null
            and refresh_token is null
            and expires_at is null
        )
        or (
            kind = 'oauth'
            and token_url is not null
            and client_id <> ''
            and client_secret <> ''
            and refresh_token <> ''
        )
    )
);
