create table organizations (
    id uuid primary key,
    name text not null,
    created_at timestamptz not null default now()
);

create table users (
    id uuid primary key,
    email text not null unique,
    name text not null,
    created_at timestamptz not null default now()
);

create table memberships (
    organization_id uuid not null references organizations (id),
    user_id uuid not null references users (id),
    role text not null,
    primary key (organization_id, user_id),
    constraint membership_role_ok check (role in ('owner', 'member'))
);

create table access_tokens (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    user_id uuid not null references users (id),
    token_hash text not null unique,
    created_at timestamptz not null default now()
);

create table teams (
    id uuid primary key,
    organization_id uuid not null references organizations (id),
    name text not null,
    unique (organization_id, name)
);

create table team_members (
    team_id uuid not null references teams (id),
    user_id uuid not null references users (id),
    primary key (team_id, user_id)
);
