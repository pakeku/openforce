# Automation

**R8 · Application runtime · Phase 4**

Depends on [events](../../kernel/events/README.md) and [jobs](../../kernel/jobs/README.md). May call [functions](../../developer-platform/functions/README.md) and outbound [webhooks](../../integrations/webhooks/README.md). Obeys [permissions](../../kernel/permissions/README.md).

Automation is the platform's workflow engine. Something happens, a condition matches, and the platform takes action. The domain model is Openforce's. n8n is the reference for how a workflow product feels, and it is not embedded.

## Why it exists

This is the capability people picture when they picture a system like Salesforce: when a deal crosses a threshold, create the task, notify someone, update the account, call the other system. Building that as scattered hooks inside the record write does not survive the second integration. Building it as trigger, condition, and action on the event stream does.

## Model

The shape a person authors:

```text
WHEN
    opportunity.amount > 100000

AND
    opportunity.stage = "Closed Won"

THEN
    create task
    notify Slack
    update account
    call webhook
```

The shape the engine runs:

```text
Trigger
  ↓
Condition
  ↓
Action
```

End to end:

```text
Event
 ↓
Trigger matcher
 ↓
Condition engine
 ↓
Workflow execution
 ↓
Actions
```

A workflow is metadata (`AutomationDefinition`) and it is versioned. Actions are the [action primitive](../../primitives/README.md#action): create, update, delete, HTTP, email, notification, and function. Branches, loops, retries, and schedules complete the model.

An approval is a workflow that waits for a person and then takes a branch. It uses the same engine. It is not a parallel product.

n8n concepts worth studying while designing the model:

```text
nodes · connections · credentials · executions
expressions · retries · webhooks · schedules
```

Credentials used by an action are the secrets held by [permissions](../../kernel/permissions/README.md) and the [connector runtime](../../integrations/connector-runtime/README.md).

### Execution history

Every run records enough to reconstruct the [inspectability](../../principles/README.md#the-whole-application-is-inspectable) trace:

```text
Opportunity 9832
      ↓
record.updated
      ↓
Workflow #182
      ↓
Condition matched
      ↓
HTTP Action
      ↓
429 Too Many Requests
      ↓
Retry 1
      ↓
Retry 2
      ↓
Dead letter
```

Retries and the dead-letter state come from [jobs](../../kernel/jobs/README.md). The workflow execution links each step to the causal event and to any [audit](../../kernel/audit/README.md) entries the actions wrote.

## Capabilities

- Match a stored event against workflow triggers.
- Evaluate conditions against the record, including field comparisons like the example above.
- Run actions on the job queue, with retries.
- Run on a schedule, not only on a record event.
- Keep an execution history a person can read when a run fails.
- Version the workflow, and show it in the "used by" list for every field it reads.
- Ask the policy engine before an action writes, and record when a workflow is allowed to exceed the triggering user's rights.

## Decisions

- The engine has its own domain model. n8n is a reference, not a dependency.
- Workflows subscribe to events. They are not invoked inside the record transaction.
- Phase 4's bar is about 80% of a simple workflow: triggers, conditions, actions, schedules, retries, and execution history, built from these primitives.

## Deferred

- A full visual node editor. The model can be authored as metadata first. A canvas is a client of that model.
- Loops over large record sets. The primitive includes loops. The first engine can run a single-record workflow well before it runs a bulk loop well.
- Embedding a third-party workflow engine.

## Related

- [Events](../../kernel/events/README.md)
- [Jobs](../../kernel/jobs/README.md)
- [Functions](../../developer-platform/functions/README.md)
- [Audit](../../kernel/audit/README.md)
