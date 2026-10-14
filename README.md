# Openforce

Openforce is an open-source business operating system. CRM is the first application written on it. The work is the platform underneath: data, metadata, workflow, a UI runtime, integrations, permissions, audit, and a way for a developer to ship a business application as configuration.

Salesforce is the existence proof that one system can run a company's records, process, and integrations. It is also priced so that plenty of teams rent less than they need. Openforce is a side project aimed at that same class of power, in the open, starting from the kernel.

The specification lives in [requirements](requirements/README.md). The kernel API is the Go service in `services/api`.

## The platform

```text
Experience         Web · Admin · Mobile · Dashboards · Reports · Builder
                        │
Runtime            Objects · Views · Forms · Search · Workflow · Automation
                        │
Kernel             Metadata · Permissions · Events · Jobs · API · Audit
                        │
Infrastructure     PostgreSQL · Redis · Object storage
```

The kernel is the part that has to be right. Objects, metadata, permissions, events, jobs, and the API live there. Screens, workflows, and dashboards are applications on top of it. Postgres, Redis, and object storage are the infrastructure, because they are enough to start.

## Twelve primitives

An application on Openforce is a combination of these, plus a version of all of them.

| Primitive | It answers |
| --- | --- |
| [Object](requirements/primitives/README.md#object) | What kinds of things exist? |
| [Field](requirements/primitives/README.md#field) | What is true of each one? |
| [Relationship](requirements/primitives/README.md#relationship) | How do they connect? |
| [View](requirements/primitives/README.md#view) | How are they browsed? |
| [Action](requirements/primitives/README.md#action) | What can happen to them? |
| [Event](requirements/primitives/README.md#event) | What just happened? |
| [Workflow](requirements/primitives/README.md#workflow) | What should happen next? |
| [Permission](requirements/primitives/README.md#permission) | Who is allowed to do it? |
| [Page](requirements/primitives/README.md#page) | How is it shown? |
| [Integration](requirements/primitives/README.md#integration) | What else does it talk to? |
| [Function](requirements/primitives/README.md#function) | Where does custom code go? |
| [Version](requirements/primitives/README.md#version) | Which revision is deployed? |

An Account is an object. A pipeline is a view. Lead routing is a workflow. A CRM ships as a strong default configuration of these primitives.

## Read this

| | |
| --- | --- |
| Why this exists, and what it is aiming at | [Vision](requirements/vision/README.md) |
| The map of every requirement | [Requirements](requirements/README.md) |
| The rules that keep the design coherent | [Principles](requirements/principles/README.md) |
| What gets built, in what order | [Roadmap](requirements/roadmap/README.md) |

## Status

Phase 1 is the kernel in `services/api`. [Issue 1](https://github.com/pakeku/openforce/issues/1) is the plan: one repo, a Go API, Postgres, and shared tables. The service defines objects, stores JSONB records, isolates them by organization, links them, and checks object-level permissions before it reads or writes a record. Deletes set `deleted_at`, and updates carry a version so two writers cannot silently overwrite each other. Phase 2 adds list filters, saved views, layouts, record pages, and the React runtime in `apps/web`. Phase 3 stores an event for each committed change to a user, an object, or a record. A field change or a deletion stores an audit job, a webhook subscription stores a delivery job, and a separate `work` command completes both. With `REDIS_URL` set, the commit publishes due jobs to Redis so that worker can wake up. Automation comes in a later phase.

## Run the kernel

Start Postgres, apply migrations, create an owner, and run the API from the repo root:

```text
docker compose up -d
go run -C services/api . migrate
go run -C services/api . seed -org Acme -email ada@example.com -name Ada
go run -C services/api .
```

The seed command prints an organization id and a bearer token once. Send it as `Authorization: Bearer <token>` on `/api/*`. `GET /healthz` is public. It returns 200 when Postgres accepts a connection, and 503 when it does not.

The default database URL is `postgres://openforce:openforce@localhost:5432/openforce?sslmode=disable`. Override it with `DATABASE_URL`. The API listens on `:8080`. Override the port with `PORT`, or the whole address with `ADDR`. Set `REDIS_URL` to `redis://localhost:6379/0` when the worker should wake from Redis. Leave it unset to keep polling Postgres.

```text
go test -C services/api ./...
```

The tests start an embedded Postgres, so Docker is not required to run them.

## Kernel API

| | |
| --- | --- |
| `GET /healthz` | Database reachability. No token. |
| `POST /api/meta/objects` | Owner defines an object and its fields. |
| `GET /api/meta/objects` | Objects in the caller's organization. |
| `GET /api/meta/objects/{name}` | One object and its fields. |
| `PUT /api/meta/objects/{name}/permissions` | Owner sets whether the member role can read and write. |
| `POST /api/meta/objects/{name}/relationships` | `belongs_to`, `has_many`, or `many_to_many`. |
| `GET /api/objects/{name}` | Live records of that object. Default limit 50, maximum 200. |
| `POST /api/objects/{name}` | Create a record. The body is the field values. Version starts at 1. |
| `GET /api/objects/{name}/{id}` | One live record. |
| `PATCH /api/objects/{name}/{id}` | `{"version":N,"data":{...}}` merges those fields. A stale version returns 409. |
| `DELETE /api/objects/{name}/{id}` | Sets `deleted_at`. Later reads omit the row. The row stays in Postgres. |
| `GET /api/objects/{name}/{id}/related/{relationship}` | Records on the other side of a relationship. |
| `POST /api/objects/{name}/{id}/related/{relationship}` | Links a many-to-many record. Body is `{"id":"..."}`. |
| `GET /api/events` | Events for the caller's organization, oldest first. |
| `GET /api/jobs` | Jobs for the caller's organization. `status` is `pending`, `done`, or `dead`. `event` is one event id. |
| `POST /api/webhooks` | Owner subscribes a URL to event names. The secret is returned once. |
| `GET /api/webhooks` | Subscriptions in the caller's organization, without secrets. |
| `DELETE /api/webhooks/{id}` | Owner removes a subscription. |
| `GET /api/objects/{name}/{id}/audit` | Field history for one record, including a deleted record. |

Field types on an object are `text`, `number`, `currency`, `date`, `boolean`, and `enum`. A `belongs_to` relationship adds a `relation` field. `formula` is not accepted in this phase. The organization owner can read and write every object. A member needs a grant.

A list accepts repeatable `filter` and `sort` parameters, such as `filter=industry:eq:SaaS&sort=employees:desc`. Operators are `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, and `contains`. Creating an object also creates an All view, a Record layout, and list and record pages. Those are edited at `/api/meta/objects/{name}/views`, `/layouts`, and `/pages`. A view named in `?view=` supplies columns, filters, and sort. A patch of a view, layout, or page sends the current version and gets the next one back.

A committed change also writes an event in that same transaction. Record creates, updates, and deletes emit `record.created`, `record.updated`, and `record.deleted`. A field that actually changes also emits `field.changed` with the previous value and the new one. Creating an object emits `object.created`, and creating a user emits `user.created`. `GET /api/events` accepts `name`, `record`, `after` (a sequence number), and `limit`. Record and field events are visible to the owner and to a member who can read that object. The other catalog names are visible to every member of the organization.

The request does not write the audit log or call a webhook. A field change or a deletion stores an audit job in the same transaction as the event. A webhook subscription stores a delivery job for each matching event in that same transaction. After that transaction commits, due jobs are appended to the Redis stream `openforce:jobs` when `REDIS_URL` is set. `go run -C services/api . work` reads the stream, then checks Postgres for jobs that are still due, and exits. The job row in Postgres keeps the status, the attempt count, and the dead letter. A job that fails waits longer before the next try, and after 5 attempts it is marked dead. `GET /api/jobs` lists the caller's jobs. `GET /api/objects/{name}/{id}/audit` returns the field history in event order, including after the record is deleted.

`POST /api/webhooks` takes `{"url":"https://example.com/hook","events":["record.created"]}`. `work` POSTs the event as JSON. The header `X-Openforce-Signature` is `sha256=` plus the hex HMAC-SHA256 of the raw body, keyed by the subscription secret. A response outside 2xx fails the job.

## Work with records

The web app in `apps/web` is the UI runtime. It renders a table and a record page from the object's view and layout, so a new object does not need its own React page.

```text
cd apps/web
npm install
npm run dev
```

Open `http://localhost:5173` and paste the seed token. The API has to be running. Set `VITE_API_URL` when it is not `http://localhost:8080`.
