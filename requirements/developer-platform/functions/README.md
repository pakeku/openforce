# Functions

**R19 · Developer platform · Phase 6**

Depends on [events](../../kernel/events/README.md) and [jobs](../../kernel/jobs/README.md). Called by [automation](../../runtime/automation/README.md) when a workflow step is a function. Subject to [permissions](../../kernel/permissions/README.md).

A function is custom code for the case a workflow cannot express. It runs in the worker environment, on an event, and it uses the platform's API to read and write records.

The first runtime is a controlled worker. It is not a general serverless platform. A sandbox comes later. Building Lambda is explicitly out of the starting design.

## Why it exists

Workflows cover a large share of business process, and they will not cover all of it. The escape hatch has to be real code, with a signature, a log, and the same audit trail as any other writer. If the escape hatch is "fork Openforce," the developer platform has a hole in it.

## Model

A function receives the event and a context:

```python
def handler(event, context):
    ...
```

```go
func Handler(ctx context.Context, event Event) error {
    ...
}
```

Both signatures are acceptable descriptions of the contract. Go and Python are both in the worker stack. The first language the platform supports is still a choice between those two. The contract is the same either way: an event in, platform APIs available through the context, errors reported back to the job.

```text
Record event
      ↓
Function
      ↓
API / DB / external service
```

Functions are files in the application project, under `functions/`, deployed with the rest of the revision. A workflow action of type `function` names one. An event can also invoke a function directly, the same way it invokes a workflow.

The function writes records through the [query API](../../kernel/query-api/README.md), so events, permissions, and audit still apply. The source channel is `Automation`, whether a workflow called the function or an event invoked it directly.

## Capabilities

- Deploy a function as part of an application revision.
- Invoke it from a workflow action or from an event.
- Give it the event payload and a context that can call the platform API.
- Run it on a worker, with the retry and dead-letter behavior of [jobs](../../kernel/jobs/README.md).
- Attribute its writes in the audit log.
- Apply the policy engine, including an explicit elevated grant when the function needs rights the triggering user does not have.

## Decisions

- The first runtime is a controlled worker in the existing worker process.
- A public multi-tenant sandbox is later. Untrusted code from arbitrary tenants is a different problem from code deployed by the application's own developer.
- Functions are versioned and deployed with the application. A function edited only on a server, outside git, is not a supported path.

## Deferred

- The language choice between Go and Python for the first supported runtime.
- Resource limits, isolation technology, and cold-start behavior of a later sandbox.
- Arbitrary outbound network access. The first runtime can call the platform API and named external services. Open egress is a sandbox decision.

## Related

- [Automation](../../runtime/automation/README.md)
- [Jobs](../../kernel/jobs/README.md)
- [Developer experience](../developer-experience/README.md)
- [Permissions](../../kernel/permissions/README.md)
