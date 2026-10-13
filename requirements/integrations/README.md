# Integrations

Integrations are the boundary between Openforce and other systems. A connector moves records and events across that boundary. Webhooks are the simplest version of the same idea: a fact goes out, or a fact comes in.

A marketplace of connectors is a later product. The requirement underneath it is a connector runtime good enough that a marketplace would have something real to list.

| ID | Requirement | Phase | Role |
| --- | --- | --- | --- |
| R11 | [Webhooks and public API](webhooks/README.md) | 3 | Outbound events in Phase 3. Inbound webhooks in Phase 5. Keys, SDKs, and OpenAPI in Phase 6. |
| R10 | [Connector runtime](connector-runtime/README.md) | 5 | OAuth, credentials, sync, mapping, polling, retries, and rate limits. |

The internal record API those connectors call is the [query and API](../kernel/query-api/README.md) requirement. This layer does not invent a second way to write a record.

## Related

- [Events](../kernel/events/README.md)
- [Jobs](../kernel/jobs/README.md)
- [Roadmap](../roadmap/README.md)
- [Requirements index](../README.md)
