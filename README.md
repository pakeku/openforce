# openforce
Decided that Salesforce is too overrated; building a new open source version (some new iteration of many open attempts). 

Yes. And I’d approach this less as “build Salesforce” and more as build a programmable business operating system, with CRM as the first vertical.

The useful mental model is:

Salesforce = data platform + metadata platform + workflow engine + UI runtime + integration platform + permissions/audit + developer platform.

Once you see it that way, you can decompose it into very normal engineering projects.

Your instincts around n8n + Docmost + Supabase/Postgres + Redis + Twenty are actually pretty good reference implementations. I would not make those dependencies of the eventual platform, though. Use them to steal ideas and validate architecture.

⸻

1. The big decomposition

I’d break the entire thing into roughly these domains:

┌─────────────────────────────────────────────────────────────┐
│                      EXPERIENCE LAYER                       │
│                                                             │
│  Web App │ Mobile │ Admin │ Dashboards │ Reports │ Builder │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│                     APPLICATION RUNTIME                      │
│                                                             │
│  CRM │ Objects │ Views │ Forms │ Search │ Activities       │
│  Workflow │ Automation │ Notifications │ Approvals         │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│                     PLATFORM KERNEL                          │
│                                                             │
│ Metadata │ Permissions │ Events │ Jobs │ API │ Audit        │
│ Versioning │ Configuration │ Tenancy │ Secrets              │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│                     INFRASTRUCTURE                           │
│                                                             │
│ PostgreSQL │ Redis │ Object Storage │ Queue │ Search        │
│ Observability │ Backups │ Secrets │ Deployment              │
└─────────────────────────────────────────────────────────────┘

The key is the platform kernel.

That’s the part I’d obsess over early, because if you get it right, everything above it becomes considerably easier.

⸻

2. Project 1 — The Object/Data Model

This is the equivalent of Salesforce’s fundamental “objects.”

Start with:

Organization
User
Team
ObjectDefinition
FieldDefinition
Record
RecordRelationship
ViewDefinition
FormDefinition

Then your CRM objects are just configurations:

Account
Contact
Lead
Opportunity
Case
Task
Event

Rather than hard-coding:

CREATE TABLE accounts (...)
CREATE TABLE contacts (...)

you eventually want:

ObjectDefinition
    ↓
FieldDefinition[]
    ↓
Record

For example:

{
  "object": "company",
  "fields": {
    "name": {
      "type": "text",
      "required": true
    },
    "employees": {
      "type": "number"
    },
    "industry": {
      "type": "select",
      "options": ["SaaS", "Finance", "Healthcare"]
    }
  }
}

Then a customer can create:

Vendor
Property
Patient
Project
Contract
Vehicle
Whatever

without you shipping new backend code.

This is one of the most important architectural decisions in the whole project.

⸻

3. Project 2 — Metadata Engine

This is separate from the data layer.

You need metadata describing:

Objects
Fields
Relationships
Views
Forms
Layouts
Buttons
Actions
Workflows
Permissions
Dashboards
Reports
Navigation

Think:

"what is the application?"
rather than:
"what data does the application contain?"

This is the thing that eventually lets you build a Salesforce-style App Builder.

I’d give it its own service/package:

/packages/metadata

with concepts like:

ObjectDefinition
FieldDefinition
RelationshipDefinition
LayoutDefinition
ViewDefinition
ActionDefinition
AutomationDefinition
PermissionDefinition

And critically:

Metadata needs versioning.

For example:

Opportunity
  v1
  v2
  v3

because eventually someone will say:

“I changed this field and now 37 workflows broke.”

That’s a platform problem, not a CRM problem.

⸻

4. Project 3 — Database Engine

I’d actually start boring.

Postgres.

Supabase is useful as an accelerator/reference implementation, but I’d architect against Postgres itself.

You need to solve:

Multi-tenancy
Transactions
Relationships
Indexes
Custom fields
Row-level permissions
Migrations
Soft deletion
Audit history
Optimistic concurrency

There’s an interesting architectural fork here.

Option A — EAV

records
fields
values

Flexible but ugly.

Option B — JSONB

records
---------
id
object_id
data jsonb
created_at
updated_at

Much more attractive initially.

Option C — dynamically generated tables

Eventually:

tenant_123.accounts
tenant_123.opportunities
tenant_123.custom_object_42

Potentially very powerful, but significantly more complicated.

I’d probably start with Postgres + JSONB + carefully indexed metadata, then evolve once actual workloads tell you where the pain is.

⸻

5. Project 4 — Query/API Engine

Once objects exist, you need a universal API.

Something conceptually like:

GET /api/objects/accounts
GET /api/objects/accounts/:id
POST /api/objects/accounts
PATCH /api/objects/accounts/:id
DELETE /api/objects/accounts/:id

But underneath:

API
 ↓
Authorization
 ↓
Metadata
 ↓
Query compiler
 ↓
Postgres

You eventually want queries like:

{
  "object": "opportunity",
  "filters": [
    {
      "field": "amount",
      "operator": "gte",
      "value": 100000
    }
  ],
  "sort": [
    {
      "field": "created_at",
      "direction": "desc"
    }
  ],
  "limit": 50
}

This becomes your SOQL-ish layer.

I’d explicitly make this a project.

⸻

6. Project 5 — UI Runtime

This is where your MERN background becomes extremely useful.

You don’t want:

AccountPage.tsx
ContactPage.tsx
OpportunityPage.tsx
...

forever.

You want:

ObjectRenderer
FieldRenderer
RecordRenderer
TableRenderer
FormRenderer
LayoutRenderer

Then:

{
  "layout": [
    {
      "component": "field",
      "field": "name"
    },
    {
      "component": "field",
      "field": "industry"
    }
  ]
}

becomes React.

So:

Metadata
    ↓
UI Schema
    ↓
React Renderer
    ↓
Application

This is basically building a low-code UI runtime.

And I would keep it separate from the CRM.

⸻

7. Project 6 — Screens / Page Builder

Then you build:

Page
 ├── Header
 ├── Tabs
 ├── Record
 ├── Table
 ├── Chart
 ├── Form
 ├── Related Records
 └── Custom Component

Something like:

{
  "page": "account-detail",
  "layout": {
    "type": "grid",
    "columns": 12,
    "children": [...]
  }
}

Now users can create:

Account page
Opportunity page
Executive dashboard
Operations dashboard
Customer portal

without you creating bespoke screens.

This is where your project starts becoming Salesforce-ish rather than “an open-source CRM.”

⸻

8. Project 7 — Event System

This deserves its own project.

Everything important should emit events.

For example:

record.created
record.updated
record.deleted
field.changed
user.created
object.created
workflow.started
workflow.completed

Architecture:

                 ┌── Workflow Engine
                 │
Record Service ──┼── Audit Log
                 │
                 ├── Notifications
                 │
                 ├── Integrations
                 │
                 └── Analytics

You don’t want:

update_record()
if workflow:
    run_workflow()
if slack:
    send_slack()
if webhook:
    send_webhook()
if audit:
    write_audit()

inside your core transaction forever.

Instead:

Record Updated
      ↓
Event Bus
 ┌────┼───────┬───────────┐
 ↓    ↓       ↓           ↓
Audit Workflow Webhook Analytics

Redis Streams, NATS, Kafka, RabbitMQ, etc. can eventually handle this.

I’d start much simpler than Kafka.

⸻

9. Project 8 — Trigger / Automation Engine

Now you can build the thing people associate with Salesforce:

WHEN
    opportunity.amount > 100000
AND
    opportunity.stage = "Closed Won"
THEN
    create task
    notify Slack
    update account
    call webhook

Model:

Trigger
  ↓
Condition
  ↓
Action

Eventually:

Event
 ↓
Trigger Matcher
 ↓
Condition Engine
 ↓
Workflow Execution
 ↓
Actions

This is where n8n becomes an excellent reference.

I’d actually study n8n’s concepts heavily:

nodes
connections
credentials
executions
expressions
retries
webhooks
schedules

But don’t embed n8n as your fundamental automation engine.

You want your own domain model.

⸻

10. Project 9 — Job System

Separate:

request/response

from:

background work

For example:

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

Workers handle:

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

Redis + a job library can get you surprisingly far.

Given your stack, I’d seriously consider:

Postgres
Redis
Go workers
Python workers

rather than immediately introducing 14 infrastructure technologies.

⸻

11. Project 10 — Integration Runtime

This is another enormous subsystem.

Break it into:

OAuth
Credentials
Connectors
Webhooks
Polling
Sync
Field Mapping
Rate Limiting
Retries
Dead Letter Queue

Then:

Integration
    ↓
Connector
    ↓
Credential
    ↓
Sync Definition

Example:

Salesforce
  ↓
HubSpot
  ↓
Postgres

or:

Stripe
  ↓
Webhook
  ↓
Integration Runtime
  ↓
Record Mutation

This is where Nango is a useful reference.

Workato is useful conceptually too, but I wouldn’t model your architecture around “the marketplace.”

First build:

A really good connector runtime.

Marketplace comes later.

⸻

12. Project 11 — API/Webhook Platform

You need two directions.

Outbound

record.created
       ↓
Webhook
       ↓
https://customer.com/webhook

Inbound

POST /webhooks/:connector/:endpoint
       ↓
verify
       ↓
normalize
       ↓
event
       ↓
workflow

Then developers can integrate with you without needing your UI.

Eventually:

REST
GraphQL
Webhooks
OAuth
API keys
Service accounts
SDKs
OpenAPI

This becomes your developer platform.

⸻

13. Project 12 — Permissions / Security

Don’t leave this until the end.

You need several distinct concepts:

Authentication
Authorization
Roles
Permissions
Teams
Groups
Ownership
Sharing
Field-level security
Record-level security
Object-level security
API permissions
Secrets
Audit

Think:

Can Alice see Accounts?
Can Alice edit Accounts?
Can Alice edit Revenue?
Can Alice see Bob's Accounts?
Can this API token modify Accounts?
Can this workflow bypass normal permissions?

I’d make authorization a central policy engine, rather than sprinkling checks everywhere.

⸻

14. Project 13 — Audit / History

This is one of those boring things that becomes absolutely fundamental.

You want:

who
what
when
before
after
why
source

For example:

2026-10-09
Alice
Opportunity 123
amount
$50,000 → $75,000
source: UI

And eventually:

UI
API
Automation
Integration
Import
Admin

all become distinguishable.

This is also incredibly useful for debugging automation.

⸻

15. Project 14 — Search

Don’t make Postgres your permanent search engine.

Eventually:

Postgres
   ↓
Search index
   ↓
OpenSearch / Elasticsearch / Meilisearch / Typesense

Search across:

Accounts
Contacts
Opportunities
Cases
Notes
Emails
Documents
Activities
Custom objects

But again: don’t build this first.

Postgres full-text search gets you surprisingly far.

⸻

16. Project 15 — Files / Documents

You need:

Object Storage
Metadata
Permissions
Versions
Previews
Attachments
Links
Document relationships

Something like:

Account
 ├── contract.pdf
 ├── proposal.pdf
 └── logo.png

S3-compatible storage is the obvious foundation.

⸻

17. Project 16 — Reporting / Analytics

Separate operational queries from analytical queries.

Initially:

Postgres
   ↓
aggregations
   ↓
charts

Eventually:

Postgres
    ↓
CDC / events
    ↓
Analytics store

Then:

Reports
Dashboards
Metrics
Funnels
Cohorts
Forecasting
BI

Salesforce’s reporting functionality is effectively another application on top of the underlying data platform.

⸻

18. Project 17 — Developer Experience

This is the one I’d put unusually high on the roadmap.

Eventually you want:

acme init
acme dev
acme deploy
acme pull
acme push
acme migrate

And:

Project
├── objects/
├── fields/
├── layouts/
├── workflows/
├── pages/
├── functions/
├── integrations/
└── permissions/

Git becomes the source of truth.

This is where you get something Salesforce itself has historically made awkward:

Infrastructure-as-code for business applications.

Imagine:

object: opportunity
fields:
  - name: amount
    type: currency
  - name: stage
    type: enum
    values:
      - prospecting
      - proposal
      - closed_won
      - closed_lost

Commit it:

git commit

Deploy it:

acme deploy production

Now you’ve got something interesting.

⸻

19. Project 18 — Version Control / Environments

I’d explicitly model:

Development
     ↓
Staging
     ↓
Production

with:

Metadata version
Schema version
Workflow version
UI version
Integration version

And:

Git
 ↓
Diff
 ↓
Migration
 ↓
Deploy

This is not just source control for code.

You’re version-controlling the entire business application.

That’s one of the areas where I think your idea could actually be substantially nicer than Salesforce.

⸻

20. Project 19 — Functions / Serverless Runtime

Eventually users will say:

“I need to do something your workflow builder can’t do.”

Give them:

def handler(event, context):
    ...

or:

func Handler(ctx context.Context, event Event) error {
    ...
}

Then:

Record event
      ↓
Function
      ↓
API / DB / external service

You can sandbox this later.

Don’t start by building Lambda.

Start with a controlled worker environment.

⸻

21. Project 20 — AI

I’d actually put AI on top of the platform, rather than making AI foundational.

Once you have:

Objects
Events
Search
Documents
Permissions
Workflows
APIs

you can build:

AI agent
AI field
AI workflow step
AI search
AI record enrichment
AI summarization
AI actions

For example:

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

That’s dramatically more interesting once the underlying primitives exist.

⸻

How I’d map your reference stack

Your proposed stack actually makes sense as a study/reference stack:

Problem	Reference
CRM UX	Twenty
Workflow UX	n8n
Docs/knowledge	Docmost
Database/backend	Supabase
Cache/queues	Redis
Integrations	Nango
Enterprise automation concepts	Workato
Frontend	React
Backend	Go
AI/data tooling	Python

But I’d draw the architecture like this:

             YOUR PLATFORM
                  │
       ┌──────────┼──────────┐
       │          │          │
   React UI      Go        Python
       │        services     │
       │          │          │
       └──────────┼──────────┘
                  │
             Postgres
                  │
          ┌───────┴───────┐
          │               │
        Redis          Object Storage
          │
       Workers
          │
     Event System
          │
 ┌────────┼─────────┐
 │        │         │
Workflow Integrations AI

And the external projects become things you can compare against, rather than dependencies.

⸻

The repo I’d probably start with

Not microservices.

A monorepo.

Something like:

/platform
│
├── apps/
│   ├── web/
│   ├── admin/
│   └── api/
│
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
│
├── services/
│   ├── api/
│   ├── worker/
│   ├── workflow/
│   ├── integrations/
│   └── notifications/
│
├── infrastructure/
│   ├── postgres/
│   ├── redis/
│   ├── storage/
│   └── terraform/
│
├── cli/
│
└── examples/
    ├── crm/
    ├── recruiting/
    └── project-management/

I would not start with:

47 microservices
Kubernetes
Kafka
Temporal
Elasticsearch
ClickHouse
Neo4j
...

That’s architecture cosplay at this stage.

You have enough engineering experience to make the boring version powerful.

⸻

The really important dependency graph

I’d build it approximately in this order:

                    ┌──────────────┐
                    │  PostgreSQL  │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │ Object Model │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │   Metadata   │
                    └──────┬───────┘
                           │
             ┌─────────────┼─────────────┐
             │             │             │
        ┌────▼────┐   ┌────▼────┐   ┌────▼────┐
        │ API     │   │ UI      │   │ AuthZ   │
        └────┬────┘   └────┬────┘   └────┬────┘
             │             │             │
             └─────────────┼─────────────┘
                           │
                    ┌──────▼───────┐
                    │ Event System │
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
             │  Platform  │
             └────────────┘

That’s your spine.

⸻

And I’d define “Salesforce” as a collection of primitives

This is probably the most useful reframing.

Don’t make your roadmap:

CRM → Marketing → Service → Commerce → etc.

Make it:

Primitive 1 — Object

Customer
Invoice
Ticket
Property
Employee

Primitive 2 — Field

text
number
currency
date
boolean
enum
relation
formula

Primitive 3 — Relationship

belongs_to
has_many
many_to_many

Primitive 4 — View

table
kanban
calendar
timeline
dashboard

Primitive 5 — Action

create
update
delete
HTTP
email
notification
function

Primitive 6 — Event

created
updated
deleted
scheduled
webhook

Primitive 7 — Workflow

trigger
conditions
actions
branches
loops
retries

Primitive 8 — Permission

who can
do what
to which records
under what conditions

Primitive 9 — Page

components
layout
data
actions

Primitive 10 — Integration

OAuth
credentials
sync
webhook
mapping
rate limits

Primitive 11 — Function

custom code

Primitive 12 — Version

everything above is deployable/versionable

That’s basically the operating system.

CRM is then:

Object:
    Account
    Contact
    Lead
    Opportunity
Views:
    Pipeline
    Accounts
    Contacts
Workflows:
    Lead routing
    Opportunity follow-up
Integrations:
    Gmail
    Slack
    Stripe

You didn’t build a CRM.

You built a platform capable of expressing one.

⸻

And there’s a very interesting “killer feature”

If I were doing this project, I’d make this one of the central design goals:

Everything should be inspectable.

Suppose a user clicks:

Opportunity → Amount

You should eventually be able to show:

Amount
────────────────────────────
Type
  Currency
Created
  2026-10-09
Used by
  ✓ Opportunity form
  ✓ Pipeline view
  ✓ Revenue dashboard
  ✓ Workflow: Enterprise Deal
  ✓ API integration: HubSpot
  ✓ AI enrichment
Permissions
  Sales: read/write
  Finance: read/write
  Contractors: hidden
History
  v1 → Currency
  v2 → Decimal
Last changed
  Alice

And if a workflow fails:

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
Dead Letter

That’s the difference between a pile of CRUD screens and a serious platform.

⸻

If I were you, I’d make the first 6 milestones

Phase 1 — Kernel

Postgres
Tenant
User
Object
Field
Record
Relationship
CRUD API

Goal:

I can define an object and create/read/update/delete records.

⸻

Phase 2 — Metadata + UI

Metadata
Dynamic forms
Dynamic tables
Layouts
Views
Filters
Sorting
Record pages

Goal:

I can create a new object without writing React code.

That’s a huge milestone.

⸻

Phase 3 — Events

Event model
Event persistence
Redis
Workers
Audit
Webhooks

Goal:

Every meaningful mutation produces a reliable event.

⸻

Phase 4 — Automation

Triggers
Conditions
Actions
Schedules
Retries
Execution history

Goal:

I can recreate 80% of the useful parts of a simple n8n workflow using my own primitives.

⸻

Phase 5 — Integrations

OAuth
Credentials
Connectors
Inbound webhooks
Outbound webhooks
Polling
Sync
Mapping
Rate limiting

Goal:

External systems become another source/sink of events and records.

⸻

Phase 6 — Developer Platform

CLI
Git
Schemas
Metadata migrations
Environments
Functions
SDK
API keys
OpenAPI

Goal:

A developer can build an entire business application on the platform without touching your core source code.

⸻

And only then I’d start aggressively adding:

CRM
Service
Marketing
Projects
ERP-ish functionality
AI
Marketplace
App Store

Because at that point they’re mostly applications built on your primitives, rather than independent products.

The part I’d be particularly interested in exploring with you is the actual kernel schema/API — e.g. what ObjectDefinition, FieldDefinition, Record, Event, Workflow, Action, Permission, and PageDefinition should look like in Postgres/Go/TypeScript. That’s where I’d expect the hardest and most consequential architectural decisions to live.