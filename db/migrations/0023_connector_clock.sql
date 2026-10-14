alter table connector_syncs
    add column every_seconds integer,
    add column next_run_at timestamptz;

alter table connector_syncs add constraint connector_clock_ok check (
    (
        every_seconds is null
        and next_run_at is null
    )
    or (
        every_seconds >= 60
        and next_run_at is not null
    )
);
