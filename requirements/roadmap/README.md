# Roadmap

Six phases, in dependency order. Each phase has one goal. Requirements listed under a phase are the ones that phase has to make real. Later requirements are specified now so the kernel does not paint them into a corner, and they are not built first.

The spine:

```text
                    ┌──────────────┐
                    │  PostgreSQL  │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │ Object model │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │   Metadata   │
                    └──────┬───────┘
                           │
             ┌─────────────┼─────────────┐
             │             │             │
        ┌────▼────┐   ┌────▼────┐   ┌────▼────┐
        │   API   │   │   UI    │   │  AuthZ  │
        └────┬────┘   └────┬────┘   └────┬────┘
             │             │             │
             └─────────────┼─────────────┘
                           │
                    ┌──────▼───────┐
                    │    Events    │
                    └──────┬───────┘
                           │
          ┌────────────────┼────────────────┐
          │                │                │
     ┌────▼────┐      ┌────▼─────┐     ┌───▼────┐
     │Workflow │      │ Webhooks │     │ Audit  │
     └────┬────┘      └────┬─────┘     └────────┘
          │                │
          └────────┬───────┘
                   │
             ┌─────▼──────┐
             │ Integrations│
             └─────┬──────┘
                   │
             ┌─────▼──────┐
             │  Developer │
             │  platform  │
             └────────────┘
```

Permissions sit on the spine before events, even though field-level and sharing rules keep growing after Phase 1. The rule is in [principles](../principles/README.md#one-policy-engine).

## Phase 1. Kernel

[Database](../kernel/database/README.md), [object model](../kernel/object-model/README.md), [query and API](../kernel/query-api/README.md), [permissions](../kernel/permissions/README.md).

```text
Postgres
Tenant
User
Object
Field
Record
Relationship
CRUD API
```

**Goal.** Define an object, then create, read, update, and delete its records.

Permissions in this phase cover authentication, organization membership, and object-level allow or deny, enforced through the policy engine. A person signs in with an email and a password and receives a bearer token. A member reads an object's catalog only when that member can read the object. Finer sharing arrives with the UI.

## Phase 2. Metadata and UI

[Metadata](../kernel/metadata/README.md), [UI runtime](../runtime/ui-runtime/README.md), [page builder](../runtime/page-builder/README.md).

```text
Metadata
Dynamic forms
Dynamic tables
Layouts
Views
Filters
Sorting
Record pages
```

**Goal.** Create a new object without writing React.

Record pages, forms, tables, filters, and sorting are the Phase 2 page builder. Dashboards, portals, and custom components use the same page model and follow once record pages work. A view is a table, a kanban grouped by an enum field, or a calendar grouped by a date field. The record list renders the board or the month, and moving a card updates the field.

## Phase 3. Events

[Events](../kernel/events/README.md), [jobs](../kernel/jobs/README.md), [audit](../kernel/audit/README.md), and the outbound half of [webhooks](../integrations/webhooks/README.md).

```text
Event model
Event persistence
Redis
Workers
Audit
Webhooks
```

**Goal.** Every meaningful mutation produces a reliable event.

Event persistence, jobs, audit, outbound webhooks, and a Redis wake-up stream are in `services/api`. A committed change writes the event in that same transaction, then publishes due jobs to Redis. `work` repeats the drain until SIGINT or SIGTERM, completes audit jobs and signed webhook deliveries, and records a dead letter after repeated failure. The job row stays in Postgres. Inbound webhooks are still ahead.

## Phase 4. Automation

[Automation](../runtime/automation/README.md).

```text
Triggers
Conditions
Actions
Schedules
Retries
Execution history
```

**Goal.** Recreate about 80% of a simple workflow with Openforce's own primitives. The domain model stays Openforce's. n8n is the reference for what that simple case includes.

A workflow definition can subscribe to `record.created`, `record.updated`, or `record.deleted`. The record request stores an automation job in that same transaction. `work` checks the conditions and can create, update, or delete a record as the triggering user. An `http` action POSTs JSON to a URL, and a response outside 2xx fails the job. A workflow does not enqueue itself for the write its own action just committed. A workflow can instead set `every`, such as `15m`. `work` records a `schedule.tick` and runs the workflow when that time arrives. The web app draws that workflow on a React Flow canvas: one trigger, then its conditions, then the actions for a match and the else actions for a miss. A notification action leaves a title and a body for a member of the organization, who reads only their own list. An email action stores the message and does not send it. Delivery to an inbox stays deferred. One approval action waits for a member: a match pauses after the earlier actions, approve continues, and reject runs else or records rejected. Loops are still ahead.

## Phase 5. Integrations

[Connector runtime](../integrations/connector-runtime/README.md), and the inbound half of [webhooks](../integrations/webhooks/README.md).

```text
OAuth
Credentials
Connectors
Inbound webhooks
Outbound webhooks
Polling
Sync
Mapping
Rate limiting
```

**Goal.** External systems become another source and sink of events and records.

An owner can register an inbound endpoint for an object. `POST /api/hooks/{id}` with `X-Openforce-Signature` creates a record as that owner, with source `Integration`, and a workflow on `record.created` sees it. An owner can store an `api_key` or `oauth` credential. The API does not return the secret, and a refresh exchanges the OAuth refresh token for a new access token. A connector sync GETs a JSON array with that credential and creates or updates records as `Integration`. A connector with `every`, such as `15m`, syncs when `work` drains due jobs, and richer mapping and rate limits are still ahead.

## Phase 6. Developer platform

[Developer experience](../developer-platform/developer-experience/README.md), [environments](../developer-platform/environments/README.md), [functions](../developer-platform/functions/README.md), plus API keys, service accounts, SDKs, and OpenAPI on the [public API](../integrations/webhooks/README.md).

```text
CLI
Git
Schemas
Metadata migrations
Environments
Functions
SDK
API keys
OpenAPI
```

**Goal.** A developer can build an entire business application on the platform without editing Openforce's core source.

An owner can issue an API key for a service account. The token is returned once, and record access follows the grants on that key. `GET /api/openapi.json` returns OpenAPI 3.0.3 for the record API the caller can read. `openforce pull` and `openforce push` copy object definitions as JSON files. `openforce setup` starts a local database, the API, and the web app from a few answers. `openforce setup` starts the worker with the API and the web app. A workflow can run a named Go function the worker already contains, and Python and a sandbox stay deferred.

## After the six phases

These are specified, and they wait until an application can be expressed on the primitives:

- [Search](../runtime/search/README.md) beyond Postgres full-text
- [Files](../runtime/files/README.md)
- [Reporting](../runtime/reporting/README.md)
- [AI](../ai/README.md)
- Further applications: service, marketing, projects, ERP-shaped modules
- A marketplace and an app store, after the connector runtime is real

Files and a basic report may want to arrive earlier if a milestone needs them. The spec keeps them out of the spine so they do not distract from it.

## Intended repository shape

A monorepo. Phase 1 has started with `services/api`, `db/migrations`, and `compose.yaml`. The rest of this tree is still the target. Those directories are created when a story needs them.

```text
/
├── apps/
│   ├── web/
│   ├── admin/
│   └── api/
├── packages/
│   ├── metadata/
│   ├── objects/
│   ├── fields/
│   ├── relationships/
│   ├── permissions/
│   ├── events/
│   ├── workflows/
│   ├── integrations/
│   ├── audit/
│   ├── search/
│   ├── files/
│   └── reporting/
├── services/
│   ├── api/
│   ├── worker/
│   ├── workflow/
│   ├── integrations/
│   └── notifications/
├── infrastructure/
│   ├── postgres/
│   ├── redis/
│   ├── storage/
│   └── terraform/
├── cli/
└── examples/
    ├── crm/
    ├── recruiting/
    └── project-management/
```

`examples/crm` is the first vertical from the [vision](../vision/README.md). Recruiting and project management are there to prove the platform is not secretly a CRM with extra tables.

## Next design work

The consequential open design is the kernel schema and API. These requirements name the concepts and the constraints. They do not yet specify tables, types, or handlers.

The types to design first, across Postgres, Go, and TypeScript:

```text
ObjectDefinition
FieldDefinition
Record
Event
Workflow
Action
Permission
PageDefinition
```

That design is the bridge from this spec to Phase 1.
