# AI

**R20 · AI · Later**

Depends on the primitives it calls: [objects](../kernel/object-model/README.md), [events](../kernel/events/README.md), [search](../runtime/search/README.md), [files](../runtime/files/README.md), [permissions](../kernel/permissions/README.md), [automation](../runtime/automation/README.md), and the [query API](../kernel/query-api/README.md).

AI sits on the platform. It is not a foundation the platform sits on. Objects, events, search, documents, permissions, workflows, and APIs come first. AI features are clients of those, the same way a report or a connector is a client.

Python is the intended tooling language. The work runs as [jobs](../kernel/jobs/README.md).

## Why it exists

Once the primitives are real, the interesting workflows are the ones that mix a model's judgment with the platform's records. Lead enrichment is the example, not the product. The product is a small set of extension points a model can occupy without growing a side door around permissions and audit.

## Model

Extension points:

```text
AI agent
AI field
AI workflow step
AI search
AI record enrichment
AI summarization
AI actions
```

An AI field is a field whose value a job fills in. An AI workflow step is an [action](../primitives/README.md#action) in a workflow. An AI action is something a person or an agent can invoke on a record. Search and summarization read records and documents the actor is already allowed to read. An agent is a longer-running job that may take several of those steps.

The illustrative flow:

```text
New Lead
   ↓
AI enrichment
   ↓
Research company
   ↓
Score lead
   ↓
Create opportunity
   ↓
Assign salesperson
   ↓
Send notification
```

Every write in that flow goes through the query API. The source channel is `Automation`. The workflow execution history shows the model step the same way it shows an HTTP step: input, output, and failure. A person can see that an AI enrichment uses a field, in that field's [inspectability](../principles/README.md#the-whole-application-is-inspectable) panel.

## Capabilities

- Run a model step as a workflow action and as a job, with retries and execution history.
- Fill an AI field asynchronously, and record the value through the normal write path.
- Enrich, summarize, and score a record using only data the invoking actor may read.
- Let an agent create and update records under an explicit permission grant.
- Show AI usage on the field and workflow surfaces that already list dependents.

## Decisions

- AI is scheduled after the primitives it depends on. It is in the "after the six phases" list on the [roadmap](../roadmap/README.md#after-the-six-phases).
- Model calls are jobs. They are not performed inside the record transaction.
- Model output that changes a record is a normal write, audited and permitted.

## Deferred

- The model provider, the prompt format, and any eval harness. Those are implementation choices made when this requirement starts.
- Agents that act without a workflow around them. The first AI surface is the workflow step and the AI field, where the platform already has a run history.

## Related

- [Automation](../runtime/automation/README.md)
- [Jobs](../kernel/jobs/README.md)
- [Search](../runtime/search/README.md)
- [Files](../runtime/files/README.md)
- [Principles](../principles/README.md#ai-is-a-client-of-the-platform)
