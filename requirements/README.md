# Requirements

Openforce is specified as twenty requirements, plus four documents that tie them together. Every requirement has its own folder and README. Nothing in this tree is implemented. The status of the whole spec is **specified**.

The identifiers R1–R20 match the original architecture notes. They are stable labels. They are not the build order. The build order is the [roadmap](roadmap/README.md).

## Read in this order

1. [Vision](vision/README.md) explains what the project is for.
2. [Primitives](primitives/README.md) names the twelve concepts every application is made of.
3. [Principles](principles/README.md) records the design rules.
4. [Roadmap](roadmap/README.md) says what is first, and what waits.
5. The [kernel](kernel/README.md) requirements are the ones that constrain everything else.

After that, read a requirement when you are working on it. Each one stands on its own and links to its neighbors.

## How the folders are grouped

```text
requirements/
  vision/                 why the project exists
  primitives/             the twelve concepts
  principles/             design rules
  roadmap/                phases and dependency order
  kernel/                 facts of the system: data, access, history, work
  runtime/                the application a person actually uses
  integrations/           the boundary with other systems
  developer-platform/     building an app without forking Openforce
  ai/                     capabilities that sit on the primitives
```

| Layer | What it is responsible for |
| --- | --- |
| [Kernel](kernel/README.md) | What objects exist, how records are stored, who may touch them, what happened, and the API over all of that. |
| [Runtime](runtime/README.md) | Turning metadata into screens, workflows, search, files, and reports. |
| [Integrations](integrations/README.md) | Connectors, credentials, sync, and webhooks. |
| [Developer platform](developer-platform/README.md) | The CLI, git, environments, and custom functions. |
| [AI](ai/README.md) | Agents, enrichment, and AI steps, after the primitives exist. |

## Index

| ID | Requirement | Layer | Phase |
| --- | --- | --- | --- |
| R1 | [Object model](kernel/object-model/README.md) | Kernel | 1 |
| R2 | [Metadata](kernel/metadata/README.md) | Kernel | 2 |
| R3 | [Database](kernel/database/README.md) | Kernel | 1 |
| R4 | [Query and API](kernel/query-api/README.md) | Kernel | 1 |
| R5 | [UI runtime](runtime/ui-runtime/README.md) | Runtime | 2 |
| R6 | [Page builder](runtime/page-builder/README.md) | Runtime | 2 |
| R7 | [Events](kernel/events/README.md) | Kernel | 3 |
| R8 | [Automation](runtime/automation/README.md) | Runtime | 4 |
| R9 | [Jobs](kernel/jobs/README.md) | Kernel | 3 |
| R10 | [Connector runtime](integrations/connector-runtime/README.md) | Integrations | 5 |
| R11 | [Webhooks and public API](integrations/webhooks/README.md) | Integrations | 3 |
| R12 | [Permissions](kernel/permissions/README.md) | Kernel | 1 |
| R13 | [Audit](kernel/audit/README.md) | Kernel | 3 |
| R14 | [Search](runtime/search/README.md) | Runtime | Later |
| R15 | [Files](runtime/files/README.md) | Runtime | Later |
| R16 | [Reporting](runtime/reporting/README.md) | Runtime | Later |
| R17 | [Developer experience](developer-platform/developer-experience/README.md) | Developer platform | 6 |
| R18 | [Environments](developer-platform/environments/README.md) | Developer platform | 6 |
| R19 | [Functions](developer-platform/functions/README.md) | Developer platform | 6 |
| R20 | [AI](ai/README.md) | AI | Later |

Phase names:

| Phase | Name | Goal, in one line |
| --- | --- | --- |
| 1 | Kernel | Define an object and perform CRUD on its records. |
| 2 | Metadata and UI | Create a new object without writing React. |
| 3 | Events | Every meaningful mutation produces a reliable event. |
| 4 | Automation | Recreate about 80% of a simple workflow from these primitives. |
| 5 | Integrations | Other systems become sources and sinks of records and events. |
| 6 | Developer platform | A developer ships an application without editing Openforce itself. |
| Later | Product capabilities | Search beyond Postgres, files, reporting, AI, and further applications. |

A few requirements span phases. The table shows where the work starts. The requirement's own README says what deepens later. Permissions (R12) start in Phase 1 on purpose: access control is part of the kernel, and it grows as views and sharing rules appear.

## Document convention

Each requirement README uses the same shape:

- **ID, layer, and phase** on the first line, then what it depends on.
- **Why it exists.**
- **Model**, the nouns and the examples.
- **Capabilities**, what the system provides.
- **Decisions**, choices already made.
- **Deferred**, work the spec explicitly postpones.
- **Related**, the neighbors.

Working assumptions are labeled. They are readings of the original notes, and they are open to revision. Where the notes left a choice open, the README leaves it open.

## Where this came from

These pages restate the original architecture notes as project requirements. The notes lived in one file, `Requirenments.md`, written as advice. Git history still has that file. This folder is the maintained spec.

Two cleanups are worth knowing about:

- The notes used `acme` as a placeholder CLI name. The command in this spec is `openforce`.
- Project numbers and the build order are different things. Search was "Project 14" and is also explicitly late. Permissions were "Project 12" and are explicitly early. Trust the phase column above.

## Glossary

| Term | Meaning |
| --- | --- |
| Organization | The tenant. Records, users, and configuration belong to one. |
| Object | A named kind of record, such as Account or a tenant-defined Vendor. |
| Field | A typed property of an object. |
| Record | One instance of an object. |
| Relationship | A belongs-to, has-many, or many-to-many link between records. |
| Metadata | The description of the application: objects, fields, views, layouts, workflows, permissions, navigation. |
| View | A way to browse records: table, kanban, calendar, timeline, dashboard. |
| Page | A composed screen: layout, components, data, and actions. |
| Action | Something the platform can do: write a record, send email, call HTTP, notify, run a function. |
| Event | A fact that already happened, such as `record.updated`. |
| Workflow | A trigger, conditions, and actions, run when an event matches. |
| Job | Work taken off the request path and run by a worker. |
| Permission | A rule about who can do what, to which records, under which conditions. |
| Integration | A configured connection to an external system. |
| Function | Custom code invoked by an event or a workflow step. |
| Version | A revision of metadata, schema, workflow, UI, or an integration, deployable between environments. |
