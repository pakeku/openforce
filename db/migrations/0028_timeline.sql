alter table view_definitions drop constraint view_kind_ok;

alter table view_definitions add constraint view_kind_ok check (kind in ('table', 'kanban', 'calendar', 'timeline'));

alter table view_definitions add column end_field text;
