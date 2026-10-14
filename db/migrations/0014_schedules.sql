alter table automation_definitions
    add column every_seconds integer,
    add column next_run_at timestamptz,
    add column actor_id uuid references users (id),
    alter column event_name drop not null,
    alter column object_id drop not null;

alter table automation_definitions drop constraint automation_event_ok;

alter table automation_definitions add constraint automation_event_ok check (
    event_name is null
    or event_name in ('record.created', 'record.updated', 'record.deleted')
);

alter table automation_definitions add constraint automation_trigger_ok check (
    (
        event_name is not null
        and object_id is not null
        and every_seconds is null
        and next_run_at is null
        and actor_id is null
    )
    or (
        event_name is null
        and object_id is null
        and every_seconds >= 60
        and next_run_at is not null
        and actor_id is not null
    )
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
    'schedule.tick'
));
