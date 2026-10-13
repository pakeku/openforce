# Vision

Openforce is a programmable business operating system. CRM is the first vertical, and the proof that the platform can express a real application. The CRM is a configuration of objects, views, workflows, and integrations. It is a separate product only in the sense that it ships with useful defaults.

## Why this exists

Salesforce showed what happens when records, automation, permissions, and integrations share one platform. It also costs more than a side project, or a lot of real companies, can justify for that power. Openforce is an open-source attempt at the same class of system.

The useful decomposition is engineering, not a product catalog:

- a data platform
- a metadata platform
- a workflow engine
- a UI runtime
- an integration platform
- permissions and audit
- a developer platform

Treating the roadmap as "CRM, then marketing, then service, then commerce" builds a pile of apps. Treating it as the primitives below builds a system those apps can be written on. Service, marketing, projects, and ERP-shaped modules come after an application can be expressed and deployed on the kernel.

## Layers

```text
┌─────────────────────────────────────────────────────────────┐
│                      EXPERIENCE LAYER                       │
│  Web app · Mobile · Admin · Dashboards · Reports · Builder  │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│                     APPLICATION RUNTIME                     │
│  Objects · Views · Forms · Search · Activities              │
│  Workflow · Automation · Notifications · Approvals          │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│                      PLATFORM KERNEL                        │
│  Metadata · Permissions · Events · Jobs · API · Audit       │
│  Versioning · Configuration · Tenancy · Secrets             │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│                      INFRASTRUCTURE                         │
│  PostgreSQL · Redis · Object storage · Queue                │
│  Observability · Backups · Secrets · Deployment             │
└─────────────────────────────────────────────────────────────┘
```

Activities, notifications, and approvals appear in the runtime layer. They are configurations the platform must be able to express: tasks and events as objects, notifications as jobs, approvals as workflows that wait on a person. They are not separate requirements.

The kernel is the layer to get right early. When objects, metadata, permissions, events, and the API are solid, the screens and the CRM defaults stay thin.

## What success looks like

A developer keeps an application in git:

```text
objects/  fields/  layouts/  workflows/  pages/
functions/  integrations/  permissions/
```

They deploy it to an environment. They do this without changing Openforce's core source.

A person using the CRM clicks a field and can see what it is, where it is used, who can read it, and how it has changed. When a workflow fails, they can see the event, the match, the action, and the retries. That inspectability is specified in the [principles](../principles/README.md).

The first proof is smaller than that. Phase 1 is done when an object can be defined and its records can be created, read, updated, and deleted. The full ladder is the [roadmap](../roadmap/README.md).

## The first application

The default CRM configuration is:

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

Those names are the first vertical. A tenant who needs `Vendor`, `Property`, `Patient`, `Project`, `Contract`, or `Vehicle` uses the same object model. Shipping that tenant's app is configuration, the same way shipping the CRM is configuration.

## Systems to study

These projects are references for a particular problem. They are not dependencies of the platform. The rule is in the [principles](../principles/README.md#study-other-systems-depend-on-few-of-them).

| Problem | Reference |
| --- | --- |
| CRM experience | Twenty |
| Workflow experience | n8n |
| Docs and knowledge | Docmost |
| Database and backend accelerator | Supabase |
| Cache and queues | Redis |
| Integrations | Nango |
| Enterprise automation concepts | Workato |
| Frontend | React |
| Backend services | Go |
| AI and data tooling | Python |

Supabase is a useful reference for how a Postgres backend feels. Openforce architectures against Postgres itself.

The intended runtime looks like this:

```text
                  Openforce
                      │
         ┌────────────┼────────────┐
         │            │            │
     React UI      Go services   Python
         │            │            │
         └────────────┼────────────┘
                      │
                  Postgres
                      │
              ┌───────┴────────┐
              │                │
            Redis        Object storage
              │
           Workers
              │
          Event system
              │
     ┌────────┼─────────┐
     │        │         │
 Workflow  Integrations  AI
```

## Related

- [Primitives](../primitives/README.md)
- [Principles](../principles/README.md)
- [Roadmap](../roadmap/README.md)
- [Requirements index](../README.md)
