alter table view_definitions drop constraint view_kind_ok;

alter table view_definitions add constraint view_kind_ok check (kind in ('table', 'kanban'));

alter table view_definitions add column group_field text;
