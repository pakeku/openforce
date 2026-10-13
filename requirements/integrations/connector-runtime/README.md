# Connector runtime

**R10 · Integrations · Phase 5**

Depends on [events](../../kernel/events/README.md), [jobs](../../kernel/jobs/README.md), [webhooks](../webhooks/README.md), and [permissions](../../kernel/permissions/README.md) for credentials. Writes records through the [query API](../../kernel/query-api/README.md).

The connector runtime is how Openforce talks to systems it does not own, on a schedule or in response to an event, with credentials, mapping, and failure handling that are the same for every connector.

Nango is the reference for this kind of runtime. Workato is a useful reference for enterprise automation ideas. The architecture here is the runtime. A marketplace of connectors comes after the runtime is good.

## Why it exists

Every integration otherwise invents its own OAuth, its own retry, and its own place to hide a token. A runtime makes "connect Stripe" and "connect a database" the same shape of configuration, with different connectors underneath.

## Model

The subsystems:

```text
OAuth
Credentials
Connectors
Webhooks
Polling
Sync
Field mapping
Rate limiting
Retries
Dead-letter queue
```

The configuration a tenant creates:

```text
Integration
    ↓
Connector
    ↓
Credential
    ↓
Sync definition
```

Two shapes the runtime has to express:

```text
Salesforce
  ↓
HubSpot
  ↓
Postgres
```

A sync that maps objects in an external CRM into Openforce records, or the other direction.

```text
Stripe
  ↓
Webhook
  ↓
Integration runtime
  ↓
Record mutation
```

An inbound event, verified and normalized, becoming a record write. The webhook admission itself is specified in [webhooks](../webhooks/README.md). The runtime owns the connector-specific verification, the mapping, and the mutation.

Credentials are secrets. They are stored under the [permissions](../../kernel/permissions/README.md) secret handling, scoped to the organization, and never written into a workflow definition or a git repository.

Rate limits, retries, and the dead-letter queue are the [jobs](../../kernel/jobs/README.md) machinery, with connector-specific limits declared by the connector.

## Capabilities

- Register a connector with a declared auth method, objects it can see, and rate limits.
- Store an OAuth or API credential for an organization, and refresh it.
- Define a sync: an external object, an Openforce object, and a field mapping.
- Run that sync on a poll or from an inbound webhook, as a job.
- Map external fields onto Openforce fields using metadata, so a renamed field is a mapping change.
- Retry within the connector's rate limit, and dead-letter what still fails, with the execution visible the way a workflow run is visible.
- Write and update records only through the query API, so permissions, events, and audit still apply. The source channel is `Integration`.

## Decisions

- The first product is the runtime, not a marketplace.
- Connectors are configuration plus a small amount of connector code. They are not one-off services with private databases.
- Record mutations from a connector take the same path as a mutation from the UI.

## Deferred

- A public connector marketplace and an app store. Listed on the [roadmap](../../roadmap/README.md#after-the-six-phases) after Phase 6.
- Bidirectional sync conflict policy beyond "the platform records both versions in audit." A default winner has to be chosen when the first real two-way sync is built. The spec requires that the conflict be visible.

## Related

- [Webhooks and public API](../webhooks/README.md)
- [Jobs](../../kernel/jobs/README.md)
- [Permissions](../../kernel/permissions/README.md)
- [Query and API](../../kernel/query-api/README.md)
