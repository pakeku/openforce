# Principles

These rules decide ambiguous design questions. Individual requirements carry the detail. When a requirement and a convenience disagree, the principle wins.

## Kernel first

Objects, metadata, permissions, events, jobs, and the API are the platform. CRM screens are a client of that platform. The [roadmap](../roadmap/README.md) builds the kernel before it builds product lines, because a weak kernel makes every later application a special case.

## Applications are configuration

A new kind of record is an object definition. A new screen is a page definition. A new process is a workflow. Handwritten `AccountPage` components and handwritten `CREATE TABLE accounts` are the exception that the [UI runtime](../runtime/ui-runtime/README.md) and the [object model](../kernel/object-model/README.md) are there to retire.

The standard CRM objects ship as that configuration, so the first vertical exercises the same path a tenant will use.

## One policy engine

Authorization lives in one place. The API, the UI, workers, and functions ask the policy engine. They do not grow their own copies of "can this user edit this field."

The engine has to answer for a person, for an API token, and for a workflow that may need to run with elevated rights. The questions are listed in [permissions](../kernel/permissions/README.md).

## Mutations become events

A record change commits, then the platform emits an event. Audit, workflows, webhooks, notifications, integrations, and analytics subscribe to that event.

The core write path stays a transaction. It does not accumulate direct calls to Slack, webhooks, and the audit log. That split is the [event](../kernel/events/README.md) requirement, and the subscribers run as [jobs](../kernel/jobs/README.md).

## Request work and background work stay apart

The API answers the request. Email, webhooks, imports, exports, sync, reports, workflow steps, and AI work run on workers. The handoff is an event, then a queue. Specified by [jobs](../kernel/jobs/README.md).

## Boring infrastructure

The first deployment is a monorepo. PostgreSQL is the system of record. Redis covers cache, streams, and jobs. Files live in S3-compatible object storage. The UI is React. Services are Go. Workers and AI tooling are Go and Python.

The following stay off the critical path until a measured limit requires them:

- a microservice per capability
- Kubernetes as a starting requirement
- Kafka as the first event bus
- Temporal, Elasticsearch, ClickHouse, and Neo4j as day-one stores

Postgres full-text search covers [search](../runtime/search/README.md) until it does not. A dedicated engine is a later decision, with OpenSearch, Elasticsearch, Meilisearch, and Typesense as the candidates.

## Study other systems, depend on few of them

Twenty, n8n, Docmost, Supabase, Nango, and Workato are references for UX and for ideas. The catalog and the reason for each one live in the [vision](../vision/README.md#systems-to-study).

Openforce keeps its own domain model for automation, connectors, and metadata. n8n's concepts worth studying are nodes, connections, credentials, executions, expressions, retries, webhooks, and schedules. Nango is the reference for a connector runtime. Workato is a reference for enterprise automation ideas, and the architecture is a connector runtime first. A marketplace comes after that runtime exists.

## The whole application is inspectable

This is a central design goal. A serious platform can explain itself. A pile of CRUD screens cannot.

A person who opens **Opportunity → Amount** should be able to see:

```text
Amount
────────────────────────────

Type
  Currency

Created
  2026-10-09

Used by
  Opportunity form
  Pipeline view
  Revenue dashboard
  Workflow: Enterprise Deal
  API integration: HubSpot
  AI enrichment

Permissions
  Sales: read/write
  Finance: read/write
  Contractors: hidden

History
  v1  Currency
  v2  Decimal

Last changed
  Alice
```

A failed workflow should read as a trace:

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

[Metadata](../kernel/metadata/README.md) records where a field is used. [Permissions](../kernel/permissions/README.md) answer the access lines. [Audit](../kernel/audit/README.md) supplies history. [Automation](../runtime/automation/README.md) supplies the execution trace. Inspectability is a requirement on those systems, not a separate screen bolted on at the end.

## The whole application is versioned

Git is the source of truth for an application: objects, fields, layouts, workflows, pages, functions, integrations, and permissions. Environments receive a revision through a diff and a migration.

A field change can break workflows, pages, and integrations. That is a platform problem, handled by [metadata](../kernel/metadata/README.md) versions and by [environments](../developer-platform/environments/README.md). Version control here covers the business application, not only the Openforce source tree. This is one of the places the project can be meaningfully better than the system it is reacting to.

## AI is a client of the platform

AI features sit on objects, events, search, documents, permissions, workflows, and APIs. They are not a foundation those pieces depend on. Specified by [AI](../ai/README.md), and scheduled after the primitives exist.

## Related

- [Vision](../vision/README.md)
- [Roadmap](../roadmap/README.md)
- [Requirements index](../README.md)
