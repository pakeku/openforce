# Webhooks and public API

**R11 · Integrations · Phase 3**

Depends on [events](../../kernel/events/README.md) for outbound delivery, and on [jobs](../../kernel/jobs/README.md) for retries. Inbound calls that change records go through the [query API](../../kernel/query-api/README.md). Developer authentication deepens under [permissions](../../kernel/permissions/README.md) in Phase 6.

This is the contract at the edge of the platform. Outbound webhooks tell another system that something happened. Inbound webhooks let another system tell Openforce. The public API, keys, and SDKs are the same edge, grown in Phase 6 so a developer can integrate without using the UI.

The internal metadata-aware API is [query and API](../../kernel/query-api/README.md). This requirement is the outside of that door.

## Why it exists

A platform other software can build on has to be reachable without its own interface. Webhooks are the smallest version of that, and they fall out of the event system almost directly. The rest of a developer platform (keys, OAuth, SDKs, a published schema) belongs in the same place so there is one edge, with one way to authenticate and one way to sign a payload.

## Model

### Outbound

Phase 3.

```text
record.created
       ↓
Webhook
       ↓
https://customer.com/webhook
```

Delivery is a job: signed payload, retries, dead letter. The event is persisted first, so a failed delivery can be replayed.

### Inbound

Phase 5, alongside the connector runtime.

```text
POST /webhooks/:connector/:endpoint
       ↓
verify
       ↓
normalize
       ↓
event
       ↓
workflow
```

Verification is connector-specific. Normalization produces an Openforce event, so workflows and audit see inbound traffic the same way they see a UI edit. The source channel is `Integration` or `API`, as [audit](../../kernel/audit/README.md) defines those channels.

### Developer surface

Phase 6.

```text
REST
GraphQL
Webhooks
OAuth
API keys
Service accounts
SDKs
OpenAPI
```

REST already exists internally as the query API. Phase 6 publishes it, authenticates it with keys and service accounts, and documents it with OpenAPI. GraphQL is part of this surface and can follow the REST publication. OAuth here is how a third-party application acts on behalf of an organization. Connector OAuth, where Openforce is the client, stays in the [connector runtime](../connector-runtime/README.md).

## Capabilities

- Subscribe an external URL to an event, and deliver a signed payload with retries.
- Accept an inbound webhook, verify it, normalize it into an event, and let workflows react.
- Identify the organization and the actor for every inbound call.
- Issue an API key and a service account whose rights the policy engine can describe.
- Publish an OpenAPI document for the record API, generated from metadata as far as the objects allow.
- Offer SDKs generated from that document.

## Decisions

- Outbound delivery rides the event and job infrastructure. There is no second delivery stack for webhooks.
- Inbound webhooks become events before they become workflow runs.
- REST is published before GraphQL. One public record API, documented, is the Phase 6 bar.

## Deferred

- GraphQL coverage of the full metadata catalog. Record CRUD is the first published surface.
- A developer portal with key management UX beyond what the admin app needs. The capability is the key and the document.

## Related

- [Events](../../kernel/events/README.md)
- [Query and API](../../kernel/query-api/README.md)
- [Connector runtime](../connector-runtime/README.md)
- [Developer experience](../../developer-platform/developer-experience/README.md)
