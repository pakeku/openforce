alter table jobs add column published_at timestamptz;

create index jobs_unpublished on jobs (created_at, id)
where status = 'pending' and published_at is null;
