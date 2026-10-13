# Query and API

**R4 · Platform kernel · Phase 1**

Depends on the [object model](../object-model/README.md), [metadata](../metadata/README.md), and [permissions](../permissions/README.md).

This is the API inside the platform. The UI, workers, functions, and integrations read and write records through it. The external developer surface (API keys, OAuth, inbound webhooks, SDKs, OpenAPI) is [webhooks and the public API](../../integrations/webhooks/README.md). Same records, two contracts. This one is metadata-aware and internal. That one is how the outside world attaches.

## Why it exists

Once any object can exist, every object needs the same API. Per-object handlers would recreate the hardcoded CRM this project is moving away from. A single pipeline resolves the object from metadata, checks permission, compiles a query, and hits Postgres.

## Model

Routes are per object name, and the set of names is metadata:

```text
GET     /api/objects/accounts
GET     /api/objects/accounts/:id
POST    /api/objects/accounts
PATCH   /api/objects/accounts/:id
DELETE  /api/objects/accounts/:id
```

`accounts` stands in for any defined object. The route shape is `/api/objects/:object`.

The pipeline under every call:

```text
API
 ↓
Authorization
 ↓
Metadata
 ↓
Query compiler
 ↓
Postgres
```

List queries carry filters, sort, and a limit. This is the SOQL-shaped layer: a structured query over metadata, compiled to SQL.

```json
{
  "object": "opportunity",
  "filters": [
    { "field": "amount", "operator": "gte", "value": 100000 }
  ],
  "sort": [
    { "field": "created_at", "direction": "desc" }
  ],
  "limit": 50
}
```

Phase 1 delivers CRUD. Filters, sort, and pagination deepen in Phase 2, alongside views.

## Capabilities

- Create, read, update, and delete a record of any defined object through one route family.
- Resolve field names and types through metadata before touching storage.
- Refuse a call the [policy engine](../permissions/README.md) denies, including field-level and record-level decisions as those land.
- Compile a filter, a sort, and a limit into a Postgres query.
- Return a stable record representation so the UI runtime and workers share one client.
- Emit the mutation to the [event](../events/README.md) system after the transaction commits. The API itself does not call workflows, webhooks, or the audit log inline.

## Decisions

- One generic object API, driven by metadata.
- Authorization runs before query compilation.
- The query language is structured data (object, filters, sort, limit), compiled to SQL. A string query language can wait.

## Deferred

- GraphQL, SDKs, and OpenAPI documents. Those are the public developer surface in Phase 6.
- Cross-object reporting queries. Operational reads of records live here. Aggregations live in [reporting](../../runtime/reporting/README.md).
- The exact operator set beyond the comparison shown above. `gte` is the illustrative operator. The compiler's operator list is part of the next API design.

## Related

- [Object model](../object-model/README.md)
- [Metadata](../metadata/README.md)
- [Permissions](../permissions/README.md)
- [Webhooks and public API](../../integrations/webhooks/README.md)
