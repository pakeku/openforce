# Search

**R14 · Application runtime · Later**

Depends on the [object model](../../kernel/object-model/README.md) and the [query API](../../kernel/query-api/README.md). Results respect [permissions](../../kernel/permissions/README.md).

Search finds records across objects. It is specified now so the query API and the record document stay indexable, and it is built after the spine. Postgres full-text search is the first implementation and will carry the product further than it first appears.

## Why it exists

Filters on one object are a query. Finding a person, a company, a note, or an email when you do not already know which object holds it is search. Those are different workloads, and the second one eventually wants its own index.

## Model

The path, once Postgres full-text is no longer enough:

```text
Postgres
   ↓
Search index
   ↓
OpenSearch · Elasticsearch · Meilisearch · Typesense
```

The corpus:

```text
Accounts
Contacts
Opportunities
Cases
Notes
Emails
Documents
Activities
Custom objects
```

Custom objects are in the corpus by default. A tenant-defined object is searchable because it is an object, with the same permission check as a CRM object. Documents join the corpus when [files](../files/README.md) exist.

The engine choice among the four candidates is open. The requirement is the index and the permission-aware query, not a specific vendor.

## Capabilities

- Search across the objects above, plus any custom object, from one query.
- Return only records the actor may see.
- Index a record from the same events that drive audit, so a write becomes searchable without a separate code path.
- Start on Postgres full-text, behind an interface a dedicated engine can replace.

## Decisions

- Postgres full-text is the first search engine.
- A dedicated engine is a later swap, chosen when Postgres is the limit.
- Search is a subscriber to record events, not a second write inside the API.

## Deferred

- The whole requirement, relative to Phases 1 through 6. Building the index interface during Phase 1 is enough.
- Ranking and relevance tuning. Correct, permission-aware results come first.

## Related

- [Query and API](../../kernel/query-api/README.md)
- [Events](../../kernel/events/README.md)
- [Files](../files/README.md)
- [Database](../../kernel/database/README.md)
