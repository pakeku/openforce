# Database

**R3 · Platform kernel · Phase 1**

Depends on PostgreSQL. Stores the [object model](../object-model/README.md). Enforces the transactions that [events](../events/README.md) are emitted after.

Openforce starts on Postgres. Supabase is a useful reference for a Postgres backend, and the schema is designed against Postgres itself.

## Why it exists

The object model promises that a tenant can invent an object without a backend release. The database is where that promise becomes a storage strategy, plus the ordinary guarantees a system of record needs: tenancy, transactions, indexes, history, and safe concurrent writes.

## What it has to solve

```text
Multi-tenancy
Transactions
Relationships
Indexes
Custom fields
Row-level permissions
Migrations
Soft deletion
Audit history
Optimistic concurrency
```

Row-level restrictions are enforced with the [permissions](../permissions/README.md) policy engine. The database has to be able to apply that decision to a query. Audit history is stored for the [audit](../audit/README.md) requirement. Migrations carry metadata and schema changes between [environments](../../developer-platform/environments/README.md).

Soft deletion keeps a deleted record available to audit and to restore. Optimistic concurrency stops two writers from silently overwriting each other.

## Record storage

Three approaches were considered.

| Approach | Shape | Place in the plan |
| --- | --- | --- |
| Entity-attribute-value | Separate tables for records, fields, and values | Flexible, and painful to query. Not the starting design. |
| JSONB documents | One row per record, values in `jsonb` | The starting design. |
| Generated tables | A physical table per object, per tenant | A possible later evolution, once real workloads show where JSONB hurts. |

The starting record shape:

```text
records
---------
id
object_id
data        jsonb
created_at
updated_at
```

Metadata that queries filter and sort on is indexed beside the document. The platform starts with Postgres, JSONB, and those indexes, and changes the physical design when workloads show the pain. The [object model](../object-model/README.md) stays stable across that change. Callers talk to the [query API](../query-api/README.md), not to the document layout.

## Capabilities

- Isolate one organization's records from another's.
- Commit a record write in a transaction, then allow an event to be emitted for that commit.
- Index the fields that filters, sorts, and relationships need.
- Store arbitrary fields declared in metadata inside the record document.
- Apply a row-level permission decision to a query.
- Soft-delete a record and keep it available to audit.
- Reject a stale write under optimistic concurrency.
- Migrate metadata and schema forward as a unit.

## Decisions

- PostgreSQL is the system of record.
- The first record storage strategy is JSONB plus carefully indexed metadata.
- Generated per-object tables, and entity-attribute-value storage, remain documented options. Adopting either one is a later decision driven by workload evidence.

## Working assumptions

- The tenant boundary is the organization from the object model. How that boundary is implemented (a tenant column, a schema per tenant, or row-level security) is part of the next schema design, and this requirement only demands that the boundary exist and be enforced.

## Deferred

- The concrete schema for `ObjectDefinition`, `FieldDefinition`, `Record`, and the rest of the kernel types. Listed under [next design work](../../roadmap/README.md#next-design-work).
- A dedicated analytics store. Operational queries stay on Postgres. See [reporting](../../runtime/reporting/README.md).
- A dedicated search index. Postgres full-text covers search until it does not. See [search](../../runtime/search/README.md).

## Related

- [Object model](../object-model/README.md)
- [Query and API](../query-api/README.md)
- [Audit](../audit/README.md)
- [Principles](../../principles/README.md#boring-infrastructure)
