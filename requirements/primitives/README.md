# Primitives

These twelve primitives are the operating system. A business application is a configuration of them. The [requirements](../README.md) specify how each one is stored, enforced, and run. This page is the short contract.

CRM, and any later vertical, is one configuration. The platform is what makes the configuration runnable.

## Object

A named kind of record.

`Customer`, `Invoice`, `Ticket`, `Property`, `Employee`, or a kind of record a tenant invents.

Specified by the [object model](../kernel/object-model/README.md).

## Field

A typed value on an object.

```text
text · number · currency · date · boolean · enum · relation · formula
```

A closed list of options, sometimes called a select or a picklist, is an enum. Specified by the [object model](../kernel/object-model/README.md) and described, versioned, and presented through [metadata](../kernel/metadata/README.md).

## Relationship

How two objects connect.

```text
belongs_to · has_many · many_to_many
```

Specified by the [object model](../kernel/object-model/README.md).

## View

How a set of records is browsed.

```text
table · kanban · calendar · timeline · dashboard
```

Specified by the [UI runtime](../runtime/ui-runtime/README.md). Dashboards that aggregate rather than list records grow into [reporting](../runtime/reporting/README.md).

## Action

Something the platform can perform.

```text
create · update · delete · HTTP · email · notification · function
```

Specified by [automation](../runtime/automation/README.md). HTTP, email, and notification actions run as [jobs](../kernel/jobs/README.md). A function action runs in the [functions](../developer-platform/functions/README.md) runtime.

## Event

A fact that has already happened.

```text
created · updated · deleted · scheduled · webhook
```

The early catalog is concrete: `record.created`, `record.updated`, `record.deleted`, `field.changed`, `user.created`, `object.created`, `workflow.started`, `workflow.completed`. Specified by [events](../kernel/events/README.md).

## Workflow

What should happen when an event matches.

```text
trigger · conditions · actions · branches · loops · retries
```

Specified by [automation](../runtime/automation/README.md).

## Permission

Who can do what.

```text
who can · do what · to which records · under what conditions
```

Specified by [permissions](../kernel/permissions/README.md). The same question applies to a person, an API token, and a workflow.

## Page

A screen composed for a purpose.

```text
components · layout · data · actions
```

A page can hold a header, tabs, a record, a table, a chart, a form, related records, and a custom component. Specified by the [page builder](../runtime/page-builder/README.md).

## Integration

A connection to another system.

```text
OAuth · credentials · sync · webhook · mapping · rate limits
```

Specified by the [connector runtime](../integrations/connector-runtime/README.md) and by [webhooks](../integrations/webhooks/README.md).

## Function

Custom code, for the case a workflow cannot express.

Specified by [functions](../developer-platform/functions/README.md). The first runtime is a controlled worker. A general serverless sandbox comes later.

## Version

Every primitive above can be revised, diffed, and deployed.

Metadata, schema, workflows, UI, and integrations each have versions. Specified by [metadata](../kernel/metadata/README.md) and by [environments](../developer-platform/environments/README.md).

## A CRM, expressed this way

```text
Objects
  Account, Contact, Lead, Opportunity, Case, Task, Event

Views
  Pipeline, Accounts, Contacts

Workflows
  Lead routing, opportunity follow-up

Integrations
  Gmail, Slack, Stripe
```

That list is an application. Adding a vertical means writing another configuration, once the primitives can run this one.

## Related

- [Vision](../vision/README.md)
- [Principles](../principles/README.md)
- [Requirements index](../README.md)
