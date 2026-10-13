# Environments

**R18 · Developer platform · Phase 6**

Depends on [metadata](../../kernel/metadata/README.md) versions. Operated through the [developer experience](../developer-experience/README.md) CLI. Migrations apply to the [database](../../kernel/database/README.md).

An environment is a running revision of a whole business application. Development, staging, and production are the same shape of thing, holding different revisions. Moving between them is a diff and a migration, not a copy of a database done by hand.

This is source control for the application. Code versioning of Openforce itself is ordinary git. This requirement is the additional claim: objects, workflows, pages, and integrations are versioned with it.

## Why it exists

A field change can break workflows, pages, and integrations. If that change is made directly in production, the breakage is a surprise. If the change is a revision that can be diffed in staging, the breakage is a review. The platform owes its users that review, because the platform is what let them define the field in the first place.

## Model

```text
Development
     ↓
Staging
     ↓
Production
```

What a revision contains:

```text
Metadata version
Schema version
Workflow version
UI version
Integration version
```

Those are slices of one deploy, not five unrelated numbers. A deploy moves them together.

```text
Git
 ↓
Diff
 ↓
Migration
 ↓
Deploy
```

The diff is readable: which object changed, which field was added, which workflow binds to a field that moved. The migration is the ordered set of metadata and schema operations that produce the target revision. Deploy applies it to one environment and records the result.

A version of an object, such as Opportunity v1 then v2 then v3, is the unit [metadata](../../kernel/metadata/README.md) already requires. Environments are where a chosen set of those versions runs.

## Capabilities

- Run development, staging, and production as isolated environments of one application.
- Name the revision each environment is on, across metadata, schema, workflows, UI, and integrations.
- Produce a diff between the git revision and the target environment.
- Produce a migration from that diff, covering metadata and schema together.
- Apply the migration to the target environment, and record who deployed what.
- Refuse a deploy that cannot explain its migration. An environment is never "edited in place" as its only history.

## Decisions

- The environment ladder is development, staging, and production.
- A deploy is git, then diff, then migration, then apply.
- One revision covers the application. A workflow is not deployed by a side channel that skips the migration.

## Deferred

- How many environments a tenant may have beyond the three. The model supports a named environment. The product default is the ladder above.
- Automatic rollback. The first requirement is a recorded migration and a previous revision to return to. One-command rollback can sit on that record.

## Related

- [Developer experience](../developer-experience/README.md)
- [Metadata](../../kernel/metadata/README.md)
- [Database](../../kernel/database/README.md)
- [Principles](../../principles/README.md#the-whole-application-is-versioned)
