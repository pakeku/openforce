# Audit

**R13 · Platform kernel · Phase 3**

Depends on [events](../events/README.md). Reads actors from [permissions](../permissions/README.md). Stored with the durability guarantees of the [database](../database/README.md).

Audit is the history of the system of record. It is also how a person debugs a workflow. Both uses require the same entry: who did what, to which field, and from where.

## Why it exists

A platform that can change records from the UI, an API, a workflow, an import, and an integration will eventually be asked why a number moved. The answer has to be reconstructable from stored history. Application logs are not that history.

## Model

Every audited change records:

```text
who · what · when · before · after · why · source
```

```text
2026-10-09
Alice
Opportunity 123
amount
$50,000 → $75,000
source: UI
```

`why` is the cause when one exists: the workflow run, the import batch, the API client, the admin action. `source` is the channel, and the channels stay distinguishable:

```text
UI · API · Automation · Integration · Import · Admin
```

Audit subscribes to the event stream. The record write does not call the audit log directly. A workflow's execution trace, specified under [automation](../../runtime/automation/README.md), links back to these entries so a failed run and a changed field can be read together. That pairing is part of [inspectability](../../principles/README.md#the-whole-application-is-inspectable).

## Capabilities

- Record a field-level change with the previous value and the new value.
- Attribute the change to a user, a token, or a workflow run.
- Distinguish the six sources above.
- Retain history for a soft-deleted record.
- Let a person read the history of one record, and the history of one workflow run.
- Write the audit entry from the event, so a path that emits a reliable event is a path that is audited.

## Decisions

- Audit is an event subscriber, written by a worker, with the event persisted first so a worker failure can be replayed.
- Field-level before and after is the unit of history. "The record changed" is not enough.
- Sources are a closed set the platform assigns. Callers do not invent a source string.

## Deferred

- Retention and export policy. The requirement is that history exists and is queryable. How long it is kept is an operational decision once real tenants exist.
- A separate analytics copy of the audit stream. [Reporting](../../runtime/reporting/README.md) may subscribe later. The audit store itself stays the operational history.

## Related

- [Events](../events/README.md)
- [Permissions](../permissions/README.md)
- [Automation](../../runtime/automation/README.md)
- [Principles](../../principles/README.md#the-whole-application-is-inspectable)
