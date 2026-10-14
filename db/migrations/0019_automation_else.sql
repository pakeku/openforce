alter table automation_definitions
    add column else_actions jsonb not null default '[]'::jsonb;

alter table automation_executions
    add column path text;

alter table automation_executions
    add constraint automation_execution_path_ok check (
        path is null or path in ('actions', 'else')
    );
