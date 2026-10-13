# Kernel

The kernel is the facts of the system. What objects exist, how their records are stored, who may touch them, what happened, and the API that exposes all of that.

Experience code calls the kernel. The kernel does not know it is running a CRM. If this layer is right, the [runtime](../runtime/README.md) stays a renderer and an automation engine over metadata.

| ID | Requirement | Phase | Role |
| --- | --- | --- | --- |
| R1 | [Object model](object-model/README.md) | 1 | Objects, fields, records, relationships, and kernel identities. |
| R3 | [Database](database/README.md) | 1 | Postgres, tenancy, transactions, and the record storage strategy. |
| R12 | [Permissions](permissions/README.md) | 1 | One policy engine for people, tokens, and workflows. |
| R4 | [Query and API](query-api/README.md) | 1 | The metadata-aware API every caller uses to read and write records. |
| R2 | [Metadata](metadata/README.md) | 2 | The description of the application, versioned. |
| R7 | [Events](events/README.md) | 3 | Mutations become facts other subsystems can trust. |
| R9 | [Jobs](jobs/README.md) | 3 | Work that must not live on the request path. |
| R13 | [Audit](audit/README.md) | 3 | Who changed what, from which source, with before and after. |

Phase 1 is R3, R1, R12, and R4, in that dependency order. Metadata (R2) is kernel too, and it is the Phase 2 unlock for the UI.

## Related

- [Roadmap](../roadmap/README.md)
- [Principles](../principles/README.md)
- [Requirements index](../README.md)
