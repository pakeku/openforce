# UI runtime

**R5 · Application runtime · Phase 2**

Depends on [metadata](../../kernel/metadata/README.md) and the [query API](../../kernel/query-api/README.md). Pages are composed on top of it by the [page builder](../page-builder/README.md).

The UI runtime is a set of generic renderers. An object does not get its own React page. A description of fields and layout goes in, and React comes out.

## Why it exists

`AccountPage.tsx`, `ContactPage.tsx`, and `OpportunityPage.tsx` do not survive contact with a tenant who adds an object the author has never heard of. The CRM screens and a tenant's screens have to be the same rendering path, or the object model is only true on the server.

## Model

Renderers:

```text
ObjectRenderer
FieldRenderer
RecordRenderer
TableRenderer
FormRenderer
LayoutRenderer
```

A layout description becomes a screen:

```json
{
  "layout": [
    { "component": "field", "field": "name" },
    { "component": "field", "field": "industry" }
  ]
}
```

```text
Metadata
    ↓
UI schema
    ↓
React renderer
    ↓
Application
```

The runtime stays independent of the CRM. It knows how to render a field of a known type. It does not know what an Opportunity is. Field widgets cover the [field primitive](../../primitives/README.md#field): text, number, currency, date, boolean, enum, relation, and, once evaluation exists, formula.

Views rendered here:

```text
table · kanban · calendar · timeline
```

Dashboards that are mostly charts sit with [reporting](../reporting/README.md) and are placed on a page by the page builder. Filters and sorting on a table use the [query API](../../kernel/query-api/README.md).

Permissions are applied before render. A field the policy engine hides is not in the schema the renderer receives.

## Capabilities

- Render a form, a table, and a record from metadata, with no per-object component.
- Render each field type with one widget, including required state and enum options.
- Render a relationship as a link to the related record.
- Apply filters and sorting by calling the query API.
- Omit fields and records the current actor may not see.
- Host a custom component only through the page builder's extension point, so the default path stays generic.

## Decisions

- React is the UI.
- The runtime is a separate part of the system from the CRM configuration.
- Per-object pages are a missing renderer, not a feature.

## Deferred

- Mobile clients. The experience layer names them. They consume the same metadata and the same API. A native client is later. The web runtime is the Phase 2 proof.
- A visual layout editor. Phase 2 can accept layout metadata from files and from a simple admin form. The fuller builder experience is the page builder.

## Related

- [Metadata](../../kernel/metadata/README.md)
- [Page builder](../page-builder/README.md)
- [Query and API](../../kernel/query-api/README.md)
- [Object model](../../kernel/object-model/README.md)
