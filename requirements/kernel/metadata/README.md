# Metadata

**R2 · Platform kernel · Phase 2**

Depends on the [object model](../object-model/README.md). Consumed by the [UI runtime](../../runtime/ui-runtime/README.md), [permissions](../permissions/README.md), [automation](../../runtime/automation/README.md), and [environments](../../developer-platform/environments/README.md).

Metadata is the description of the application. Records are what the application contains. The split is what makes an app builder possible: change the description, and the running application changes with it.

## Why it exists

Objects and fields are necessary and not sufficient. A platform also has to describe how those objects are laid out, listed, acted on, automated, permitted, and navigated. That description needs its own model, its own package, and its own versions. A field edit that breaks thirty-seven workflows is a platform failure, and it is a metadata problem.

## Model

The metadata catalog covers:

```text
Objects          Fields           Relationships
Views            Forms            Layouts
Buttons          Actions          Workflows
Permissions      Dashboards       Reports
Navigation
```

The definition types:

```text
ObjectDefinition
FieldDefinition
RelationshipDefinition
LayoutDefinition
ViewDefinition
ActionDefinition
AutomationDefinition
PermissionDefinition
```

Page definitions join this catalog through the [page builder](../../runtime/page-builder/README.md). They are the same kind of artifact: a versioned description of a screen.

Intended home, once the repository grows past documentation: `packages/metadata`. The target tree is in the [roadmap](../../roadmap/README.md#intended-repository-shape).

### Versions

Every definition is versioned.

```text
Opportunity
  v1
  v2
  v3
```

A published version is what views, workflows, integrations, and API clients bind to. Editing a field produces a new version and a list of dependents, which is the usage half of [inspectability](../../principles/README.md#the-whole-application-is-inspectable).

## Capabilities

- Store the catalog above as data, separate from record storage.
- Version an object and its fields so a change is a revision, and dependents can be found.
- Answer "where is this field used?" across forms, views, dashboards, workflows, and integrations.
- Feed the UI runtime a description it can render without a per-object React page.
- Feed the developer platform a description it can diff and deploy. See [environments](../../developer-platform/environments/README.md).

## Decisions

- Metadata is its own subsystem. It is not a handful of columns on the record table.
- Versions are mandatory. An unversioned edit of a field, layout, or workflow is incomplete.
- The CRM's objects, views, and workflows are metadata, shipped as the default application.

## Deferred

- The file format a developer commits. The [developer experience](../../developer-platform/developer-experience/README.md) shows a declarative shape. JSON and YAML are both compatible with that shape, and the choice is open.
- Migration mechanics between versions. The pipeline is git, diff, migration, deploy, specified under environments.

## Related

- [Object model](../object-model/README.md)
- [UI runtime](../../runtime/ui-runtime/README.md)
- [Environments](../../developer-platform/environments/README.md)
- [Principles](../../principles/README.md#the-whole-application-is-versioned)
