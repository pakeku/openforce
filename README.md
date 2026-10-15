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

Phase 1 is the kernel in `services/api`. [Issue 1](https://github.com/pakeku/openforce/issues/1) is the plan: one repo, a Go API, Postgres, and shared tables. The service defines objects, stores JSONB records, isolates them by organization, links them, and checks object-level permissions before it reads or writes a record. Deletes set `deleted_at`, and updates carry a version so two writers cannot silently overwrite each other. Phase 2 adds list filters, saved views, layouts, record pages, and the React runtime in `apps/web`. Phase 3 stores an event for each committed change to a user, an object, or a record. A field change or a deletion stores an audit job, a webhook subscription stores a delivery job, and a separate `work` command completes both. With `REDIS_URL` set, the commit publishes due jobs to Redis so that worker can wake up. A workflow can subscribe to a record event, and that same worker runs it after the commit.

## Run the kernel

From the repository, `openforce setup` asks for an organization, owner, and password, starts a local Postgres, applies migrations, seeds that owner, and starts the API, the worker, and the web app. The answers, except the password, are saved in `.openforce/dev.json`. The next run can reuse them and skip seeding.

```text
go run -C cli . setup
```

Press Ctrl+C to stop the worker, the API, the web app, and the local database. Pass `-database postgres://...` to use a Postgres you already run, including `docker compose up -d`. `-reuse` starts again from the saved file.

The same steps by hand:

```text
docker compose up -d
go run -C services/api . migrate
go run -C services/api . seed -org Acme -email ada@example.com -name Ada
go run -C services/api .
```

The seed command prints an organization id and a bearer token once. Optional `-password` stores a bcrypt hash for that owner so the same email can sign in. The command still prints the token and does not print the password. Without `-password`, the hash stays null. Send the token as `Authorization: Bearer <token>` on `/api/*`. `GET /healthz` is public. It returns 200 when Postgres accepts a connection, and 503 when it does not.

The default database URL is `postgres://openforce:openforce@localhost:5432/openforce?sslmode=disable`. Override it with `DATABASE_URL`. The API listens on `:8080`. Override the port with `PORT`, or the whole address with `ADDR`. Set `REDIS_URL` to `redis://localhost:6379/0` when the worker should wake from Redis. Leave it unset to keep polling Postgres.

```text
go test -C services/api ./...
```

The tests start an embedded Postgres, so Docker is not required to run them.

## Kernel API

| | |
| --- | --- |
| `GET /healthz` | Database reachability. No token. |
| `POST /api/login` | Public. Email and password return a bearer token and an organization id. |
| `POST /api/members` | Owner creates a member with a password. No token in the response. |
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
| `GET /api/openapi.json` | OpenAPI 3.0.3 for record CRUD on the objects the caller can read. |
| `GET /api/objects/{name}/{id}/related/{relationship}` | Records on the other side of a relationship. |
| `POST /api/objects/{name}/{id}/related/{relationship}` | Links a many-to-many record. Body is `{"id":"..."}`. |
| `GET /api/events` | Events for the caller's organization, oldest first. |
| `GET /api/jobs` | Jobs for the caller's organization. `status` is `pending`, `done`, or `dead`. `event` is one event id. |
| `POST /api/webhooks` | Owner subscribes a URL to event names. The secret is returned once. |
| `GET /api/webhooks` | Subscriptions in the caller's organization, without secrets. |
| `DELETE /api/webhooks/{id}` | Owner removes a subscription. |
| `POST /api/inbound` | Owner registers an inbound endpoint for an object. The secret is returned once. |
| `GET /api/inbound` | Inbound endpoints in the caller's organization, without secrets. |
| `DELETE /api/inbound/{name}` | Owner removes an inbound endpoint. |
| `POST /api/hooks/{id}` | Signed body becomes a record. No bearer token. |
| `POST /api/credentials` | Owner stores an `api_key` or `oauth` credential. The secret is not returned. |
| `GET /api/credentials` | Credential names in the caller's organization, without secrets. |
| `PATCH /api/credentials/{name}` | Owner replaces an API key, or an OAuth client secret or refresh token. |
| `POST /api/credentials/{name}/refresh` | Owner exchanges an OAuth refresh token for a new access token. |
| `DELETE /api/credentials/{name}` | Owner removes a credential. |
| `POST /api/connectors` | Owner maps a URL onto an object and a stored credential. Optional `every` runs the sync on a clock. |
| `GET /api/connectors` | Connectors in the caller's organization. |
| `POST /api/connectors/{name}/sync` | Stores a sync job. `work` GETs the URL and writes records. |
| `DELETE /api/connectors/{name}` | Owner removes a connector. Records it already wrote stay. |
| `POST /api/keys` | Owner issues an API key. The token is returned once. |
| `GET /api/keys` | API keys in the caller's organization, without tokens. |
| `PATCH /api/keys/{name}` | Owner replaces the grants and keeps the token. |
| `DELETE /api/keys/{name}` | Owner revokes an API key. |
| `GET /api/objects/{name}/{id}/audit` | Field history for one record, including a deleted record. |
| `POST /api/automations` | Owner defines a workflow on a record event. |
| `GET /api/automations` | Workflows in the caller's organization. |
| `PATCH /api/automations/{name}` | Owner sends the current version and gets the next one back. |
| `DELETE /api/automations/{name}` | Owner removes a workflow and its pending jobs. |
| `GET /api/automations/{name}/executions` | Whether each matching event was skipped, waiting, rejected, or done. |
| `GET /api/emails` | Owner lists stored outbound email. The message is stored, not sent. |
| `GET /api/approvals` | The caller's pending approvals, newest first. A service key cannot call it. |
| `POST /api/approvals/{id}` | That user sends approve or reject. The decision continues the workflow. |

`POST /api/login` takes `email` and `password`. One membership returns 200 `{token, organization}` for that organization. More than one membership also requires `organization`, the organization id, or the response is 400 `organization is required`. An unknown email, a wrong password, and a user with no password are all 401 `unauthorized`. The body does not say which part failed. The token is a new bearer token and authenticates as that user. `POST /api/members` is owner-only. The body is `name`, `email`, and `password`. A password shorter than 8 characters, or a missing password, is 400 `password is required`. The email is stored lowercase. A duplicate email is 409. The 201 response is `email`, `name`, and `role` `member`. It returns no token and no password. A member or a service key receives 403. There is no public signup, no password-reset email, and no external identity provider. Programs still use API keys.

Field types on an object are `text`, `number`, `currency`, `date`, `boolean`, and `enum`. A `belongs_to` relationship adds a `relation` field. `formula` is not accepted in this phase. The organization owner can read and write every object. A member needs a grant. An API key uses the grants stored on that key. A member lists objects and reads their views, layouts, pages, and relationships only when that member can read the object.

A list accepts repeatable `filter` and `sort` parameters, such as `filter=industry:eq:SaaS&sort=employees:desc`. Operators are `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, and `contains`. Creating an object also creates an All view, a Record layout, and list and record pages. Those are edited at `/api/meta/objects/{name}/views`, `/layouts`, and `/pages`. A view named in `?view=` supplies columns, filters, and sort. A patch of a view, layout, or page sends the current version and gets the next one back.

A committed change also writes an event in that same transaction. Record creates, updates, and deletes emit `record.created`, `record.updated`, and `record.deleted`. A field that actually changes also emits `field.changed` with the previous value and the new one. Creating an object emits `object.created`, and creating a user emits `user.created`. `GET /api/events` accepts `name`, `record`, `after` (a sequence number), and `limit`. Record and field events are visible to the owner, to a member who can read that object, and to an API key with a read grant on that object. The other catalog names are visible to every member of the organization. An API key does not see those other names.

The request does not write the audit log or call a webhook. A field change or a deletion stores an audit job in the same transaction as the event. A webhook subscription stores a delivery job for each matching event in that same transaction. After that transaction commits, due jobs are appended to the Redis stream `openforce:jobs` when `REDIS_URL` is set. `go run -C services/api . work` keeps running until SIGINT or SIGTERM. Each pass enqueues due schedules and due connectors, publishes due jobs, reads the stream, then checks Postgres for jobs that are still due. `go run -C services/api . work -once` runs one pass, prints `delivered N`, and exits. `N` counts jobs read from the stream and jobs still due in Postgres, not schedules enqueued on that pass. When `REDIS_URL` is unset, an idle pass waits one second and drains again. When it is set, the stream read waits about one second and does not wait forever. A database error exits non-zero. After the pass already running finishes, a signal exits 0. The job row in Postgres keeps the status, the attempt count, and the dead letter. A job that fails waits longer before the next try, and after 5 attempts it is marked dead. `GET /api/jobs` lists the caller's jobs. `GET /api/objects/{name}/{id}/audit` returns the field history in event order, including after the record is deleted.

`POST /api/webhooks` takes `{"url":"https://example.com/hook","events":["record.created"]}`. `work` POSTs the event as JSON. The header `X-Openforce-Signature` is `sha256=` plus the hex HMAC-SHA256 of the raw body, keyed by the subscription secret. A response outside 2xx fails the job.

`POST /api/inbound` takes `{"name":"leads","object":"company"}` and returns a secret once. `POST /api/hooks/{id}` reads that same header. A matching signature creates a record as the owner who registered the endpoint, with source `Integration`. The body is the field values. A workflow on `record.created` sees that record when `work` runs. A wrong signature is rejected and stores nothing.

`POST /api/credentials` stores a secret for the organization. An `api_key` body is `{"name":"stripe","label":"Stripe","kind":"api_key","secret":"..."}`. An `oauth` body also sends `token_url`, `client_id`, `client_secret`, and `refresh_token`. List and get return the name, label, and kind. They never return the secret, the client secret, the refresh token, or the access token. `POST /api/credentials/{name}/refresh` sends the refresh token to `token_url` and stores the new access token. A failed refresh leaves the stored credential unchanged.

`POST /api/connectors` takes a name, a label, a credential, an object, a URL, an `external_id`, and a `mapping` from remote fields to object fields. Optional `every` uses the same values as a workflow schedule: `15m`, `1h`, or `1d`, at least one minute and at most 366 days. An invalid `every` is rejected. Omitting it leaves the connector manual, and the clock does not sync it. When `every` is set, the next run starts due. `work` stores a `sync.requested` job and moves the next run forward by `every`. A second pass before that time does not sync again. `GET` returns `every` and `next_run_at` when a clock is set, and omits both when it is not. `POST /api/connectors/{name}/sync` still stores one sync. `work` GETs the URL with `Authorization: Bearer` and the credential's secret. The body is a JSON array of objects. A new external id creates a record with source `Integration`. The same external id updates that record. A response outside 2xx fails the job and writes nothing for that attempt.

`POST /api/keys` takes a name, a label, and `grants`. Each grant names an object and sets `can_read` and `can_write`. The response includes `token` once. List and get return the name, the label, and the grants, and they never return the token. A call with that token is a service account. It can read and write records only where a grant allows, and it cannot call owner or catalog routes. It does not inherit the owner's access. A workflow started by that call runs as the key. `PATCH /api/keys/{name}` replaces the grants and keeps the token. `DELETE` revokes it.

`GET /api/openapi.json` returns OpenAPI 3.0.3 for record CRUD. An owner sees every object in the organization. A member or a service API key sees objects where that caller has `can_read`. No readable objects yields a valid document whose `paths` object is empty, and another organization's objects are absent. List documents `limit` (1 to 200, default 50), repeatable `filter` (`field:op:value`), repeatable `sort` (`field:asc` or `field:desc`), and `view`. The create body is the field object and the record response is `id`, `version`, and `data`. A patch body sends `version` and partial `data`, and `version` is required. Delete is an empty 204. Text and relation are strings, number and currency are numbers, boolean is a boolean, date is a string, and enum is a string whose `enum` lists the options. Security is HTTP bearer. Responses include 401, 403, 404, and 409 where those statuses are real. The document does not describe metadata, workflows, credentials, or webhooks. A service key may call this route and stays forbidden on owner and catalog routes.

`POST /api/automations` takes a name, a label, an event (`record.created`, `record.updated`, or `record.deleted`), an object, conditions, and actions. Conditions use the same operators as a list filter and are combined with and. Actions are `create`, `update`, `delete`, and `http`. A value `{"field":"name"}` copies a field from the triggering record, and `{"record":true}` copies that record's id. An `http` action POSTs JSON to a URL. With `data` set, that object is the body after those references are filled in. Without `data`, the body carries the event name, the object, the record id, and the record. A response outside 2xx fails the job. A workflow can run on a schedule instead of a record event. `every` is at least one minute, written as `15m`, `1h`, or `1d`. `work` stores a `schedule.tick` event and runs that workflow when the time arrives, then sets the next run. A schedule can create a record or POST JSON. Update and delete stay on record events, because a schedule has no triggering record. `update` and `delete` change the triggering record unless `record` points at another one. The record request stores an automation job in the same transaction as the event. `work` checks the conditions and runs the actions as the user who caused the event. A miss is an execution with status `skipped`. A finished run is `done`. The policy check still applies to each write. A workflow does not enqueue itself for the write its own action just committed.

An `approval` action is `{"type":"approval","user":"<email>"}` and it belongs in `actions`. The email is a member of the organization, or the save is 400 `not found`. One workflow may contain one approval. A second approval, or an approval in `else`, is 400 `invalid action`. A condition miss with an empty `else` stays `skipped`. A miss with `else` runs those actions and does not wait. A match runs the actions before the approval, writes an execution with status `waiting` and no path, and completes the job. Actions after the approval wait. `GET /api/approvals` returns the caller's pending rows, newest first: `id`, the workflow `name`, and `created_at`. It does not return anyone else's rows. A service key cannot call it. `POST /api/approvals/{id}` takes `{"decision":"approve"}` or `{"decision":"reject"}` from that user. Anyone else receives 403. Approve runs the later actions, sets status `done`, and sets `path` to `actions`. Reject runs `else` when it is non-empty, with status `done` and path `else`. Reject with an empty `else` sets status `rejected` and runs nothing. A second decision is 409 `already decided`. The continuation happens in that POST and does not enqueue a second automation job for the same event. If a later action fails, the response is that error, the execution stays `waiting`, and no due job is left. `created` lists the records the steps that ran created.

An `email` action stores a message. It does not contact an SMTP server, and the message is stored, not sent. `to` and `subject` are required. `body` may be empty. Each of those three is a literal string or `{"field":"name"}`. `{"record":true}` is invalid on them, because the value is text and not a record id. A schedule cannot use a field reference. `work` inserts the row and completes the job. A later pass does not store a second row for the same execution. `GET /api/emails` returns `to`, `subject`, `body`, and `created_at` to the owner. A member receives 403.

## CLI

The `openforce` command in `cli` talks to the API over HTTP. `pull` writes `objects/<name>.json` for every object from `GET /api/meta/objects`. Each file has `name`, `label`, and `fields` (`name`, `type`, `required`, and `options` when the type is `enum`). `push` creates a missing object with `POST /api/meta/objects`. An object whose file matches the live definition is left unchanged. An object that differs exits non-zero and the message names that object. Objects that are not in the directory are not deleted.

```text
go run -C cli . pull -url http://localhost:8080 -token <token> -dir <project>
go run -C cli . push -url http://localhost:8080 -token <token> -dir <project>
```

A missing `-url`, `-token`, or `-dir` exits 2 and names the flag. An API error is printed and the process exits 1.

## Work with records

The web app in `apps/web` is the UI runtime. It renders a table and a record page from the object's view and layout, so a new object does not need its own React page. The record page lists that audit response. The Flows screen draws a workflow with [React Flow](https://reactflow.dev/learn) and saves that chain through the automation API, including one approval on the match path. The Approvals screen lists the caller's pending approvals and posts approve or reject. The signed-in header opens Notifications for `GET /api/notifications`. Home is the page after sign-in and summarizes approvals, notifications, and objects.

```text
cd apps/web
npm install
npm run dev
```

Open `http://localhost:5173`. When no token is stored, sign in with email and password, or paste the seed token. The API has to be running. Set `VITE_API_URL` when it is not `http://localhost:8080`.
