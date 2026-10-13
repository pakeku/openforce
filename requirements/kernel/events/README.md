# Events

**R7 · Platform kernel · Phase 3**

Depends on the [query API](../query-api/README.md) committing a mutation first. Consumed by [automation](../../runtime/automation/README.md), [audit](../audit/README.md), [webhooks](../../integrations/webhooks/README.md), [jobs](../jobs/README.md), and later by [reporting](../../runtime/reporting/README.md) and [AI](../../ai/README.md).

An event is a fact that has already happened. Subscribers react to it. The code that wrote the record does not call those subscribers.

## Why it exists

Putting workflow runs, Slack messages, webhooks, and audit writes inside the request that updated a record couples the system of record to every side effect. One of them being slow or down would then threaten the write. The write commits. The event is the contract everyone else sees.

## Model

The early catalog:

```text
record.created
record.updated
record.deleted

field.changed

user.created

object.created

workflow.started
workflow.completed
```

`field.changed` carries the field, the previous value, and the new value, so audit and workflows can match on a specific change. The catalog grows (scheduled ticks, inbound webhooks) and these names stay stable.

Fan-out:

```text
                 ┌── Workflow engine
                 │
Record service ──┼── Audit log
                 │
                 ├── Notifications
                 │
                 ├── Integrations
                 └── Analytics
```

The record service's only extra duty is to emit. Delivery to each subscriber is a [job](../jobs/README.md).

```text
Record updated
      ↓
Event bus
 ┌────┼────────┬───────────┐
 ↓    ↓        ↓           ↓
Audit Workflow Webhook   Analytics
```

Events are persisted before subscribers run, so a failed subscriber can be retried from the stored event. Phase 3's goal is that every meaningful mutation produces a reliable event. "Meaningful" means a committed change to a record, a field, a user, an object definition, or a workflow's start and finish.

## Capabilities

- Emit the catalog above from the kernel after the relevant transaction commits.
- Persist the event before any subscriber runs.
- Deliver one stored event to many subscribers.
- Retry a subscriber without re-running the original transaction.
- Let a subscriber identify the organization, the object, the record, and the actor.

## Decisions

- Side effects subscribe. They are not called from the write path.
- The first transport is the simple one Postgres and Redis can already support. Redis Streams is the likely fit, because Redis is already in the stack for jobs.
- Kafka, NATS, and RabbitMQ stay available if volume outgrows that transport. They are not part of Phase 3.

## Deferred

- A public event schema for third parties. Internal subscribers use the stored event. External delivery is an outbound [webhook](../../integrations/webhooks/README.md), which has its own payload.
- Exactly-once side effects. The guarantee aimed at here is a persisted event and retried delivery. Subscribers have to tolerate a repeated delivery.

## Related

- [Jobs](../jobs/README.md)
- [Audit](../audit/README.md)
- [Automation](../../runtime/automation/README.md)
- [Principles](../../principles/README.md#mutations-become-events)
