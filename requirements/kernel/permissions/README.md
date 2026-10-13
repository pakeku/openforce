# Permissions

**R12 · Platform kernel · Phase 1**

Depends on the [object model](../object-model/README.md) for organizations, users, and teams. Consulted by the [query API](../query-api/README.md), the [UI runtime](../../runtime/ui-runtime/README.md), [jobs](../jobs/README.md), and [functions](../../developer-platform/functions/README.md).

Access control starts with the kernel. It is not a layer added after the CRM screens exist.

## Why it exists

Every later feature asks a variation of the same question: may this actor do this to this data. Sprinkling that check through handlers guarantees the checks will disagree. One policy engine is the [principle](../../principles/README.md#one-policy-engine).

## Model

The vocabulary:

```text
Authentication
Authorization
Roles
Permissions
Teams
Groups
Ownership
Sharing
Field-level security
Record-level security
Object-level security
API permissions
Secrets
Audit
```

Authentication establishes the actor. Authorization is the policy decision. Secrets (integration credentials, API tokens) are stored and scoped here, and used by the [connector runtime](../../integrations/connector-runtime/README.md). Audit of permission changes belongs to [audit](../audit/README.md).

The engine has to answer at least:

```text
Can Alice see Accounts?
Can Alice edit Accounts?
Can Alice edit Revenue?
Can Alice see Bob's Accounts?
Can this API token modify Accounts?
Can this workflow bypass normal permissions?
```

Those six questions are object-level read, object-level write, field-level write, record-level read (sharing and ownership), API-client rights, and elevated workflow rights.

Phase 1 answers the first two for a user inside an organization, and refuses everyone else. Field-level, record-level, token, and workflow elevation deepen in Phase 2 as views and sharing rules appear, and again in Phase 6 when API keys and functions ship. The engine's location does not move. Callers keep asking it.

## Capabilities

- Authenticate a user and know which organization they are acting in.
- Decide object-level read and write for a user.
- Decide field-level visibility and edit rights. A hidden field is absent from API responses and from the UI schema.
- Decide record-level access from ownership, team, and sharing rules.
- Decide what an API token or service account may do, separately from what its owner may do in the UI.
- Decide whether a workflow or function may run with rights beyond the user who triggered it, and record that it did.
- Expose one decision point the API, the UI, and workers all call.

## Decisions

- Authorization is a central policy engine.
- Object, field, and record are three different scopes, and a grant at one scope does not imply a grant at the others.
- Workflow elevation is an explicit policy, visible in the [audit](../audit/README.md) trail. Silent bypass is a bug.

## Deferred

- The policy language itself (roles only, or roles plus conditional expressions). The six questions are the acceptance tests. The syntax is part of the next design pass.
- A full sharing-rule builder in the UI. The engine comes first.

## Related

- [Query and API](../query-api/README.md)
- [Audit](../audit/README.md)
- [Metadata](../metadata/README.md)
- [Principles](../../principles/README.md#one-policy-engine)
