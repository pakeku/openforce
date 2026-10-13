# Jobs

**R9 · Platform kernel · Phase 3**

Depends on [events](../events/README.md) for the handoff, and on Redis for the queue. Runs the work of [automation](../../runtime/automation/README.md), [webhooks](../../integrations/webhooks/README.md), [audit](../audit/README.md), and the [connector runtime](../../integrations/connector-runtime/README.md).

A job is work that must not live inside the HTTP request. The API commits and returns. A worker does the rest.

## Why it exists

Email, sync, imports, and workflow steps are slower and less reliable than a record write. Mixing them into the request makes the product feel broken whenever a third party is slow. The boundary is deliberate:

```text
POST /opportunity
      ↓
API
      ↓
DB transaction
      ↓
event
      ↓
queue
      ↓
worker
```

## Model

Workers handle:

```text
email
webhooks
imports
exports
AI jobs
workflow execution
sync jobs
reports
large queries
notifications
scheduled automation
```

AI jobs and heavy reports exist in the model so the queue can carry them later. Phase 3 stands up the queue, the worker, audit delivery, and outbound webhooks. The rest arrive with the requirement that produces them.

Redis plus a job library is enough. Workers are Go, Python, or both. The stack allows both, because services lean Go and AI tooling leans Python. Which language runs which job is an implementation choice, constrained by one queue and one event shape.

## Capabilities

- Accept work from a committed event without holding the request open.
- Run the job kinds above as they come online.
- Retry a failed job with backoff.
- Park a job that has exhausted its retries, so a person can see it. The dead-letter state in the [inspectability](../../principles/README.md#the-whole-application-is-inspectable) trace depends on this.
- Schedule a job for later, which is how [automation](../../runtime/automation/README.md) expresses schedules.
- Attribute the job to the event and the organization that caused it.

## Decisions

- Request handling and background work are separate processes.
- Redis is the first queue.
- The worker pool stays small. Go and Python are both allowed. A new infrastructure technology is not required to get a worker running.

## Deferred

- A workflow engine with its own orchestrator (Temporal and similar). Scheduled and retried jobs cover Phase 3 and Phase 4. A dedicated orchestrator is justified by a limit this queue actually hits.
- Fairness and multi-tenant quotas on the queue. Worth designing once more than one real tenant is generating work.

## Related

- [Events](../events/README.md)
- [Automation](../../runtime/automation/README.md)
- [Webhooks and public API](../../integrations/webhooks/README.md)
- [Principles](../../principles/README.md#request-work-and-background-work-stay-apart)
