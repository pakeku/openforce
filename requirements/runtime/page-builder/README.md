# Page builder

**R6 · Application runtime · Phase 2**

Depends on the [UI runtime](../ui-runtime/README.md). Page definitions are versioned [metadata](../../kernel/metadata/README.md).

A page is a composition of components over data the platform already understands. Record pages are the Phase 2 milestone. Dashboards, portals, and custom components are the same model, built after record pages work.

This is the point where the project starts to feel like a platform a person can shape, rather than an open-source CRM with a fixed set of screens.

## Why it exists

The UI runtime can render a form and a table. A usable application is a screen with a header, related records, and a place for the work a role actually does. If those screens are handwritten, every tenant variation is a fork. If they are pages, they are configuration.

## Model

```text
Page
 ├── Header
 ├── Tabs
 ├── Record
 ├── Table
 ├── Chart
 ├── Form
 ├── Related records
 └── Custom component
```

```json
{
  "page": "account-detail",
  "layout": {
    "type": "grid",
    "columns": 12,
    "children": []
  }
}
```

Pages a tenant can create from that model:

```text
Account page
Opportunity page
Executive dashboard
Operations dashboard
Customer portal
```

The account and opportunity pages are Phase 2. The executive dashboard, the operations dashboard, and the customer portal are the same document once charts, permissions for external users, and custom components exist. They are specified now so page JSON does not grow a one-off structure for each of them.

A custom component is an extension registered by the application, referenced by name from the page. It is how a page holds something the built-in components do not cover, and it is used sparingly. The default components cover the list above.

## Capabilities

- Define a page as a grid of the components above, stored as metadata.
- Bind a page to an object, so a record page knows which record it is showing.
- Place a form, a table, and a list of related records on that page using the UI runtime.
- Version the page with the rest of the application's metadata.
- Serve the CRM's default record pages as shipped page definitions.

## Decisions

- A page is data. The page builder stores a document the UI runtime already knows how to render.
- The grid is 12 columns, so layouts have one shared measurement.
- Phase 2 scope is the record page: header, fields, related records, and a form. Chart, portal, and custom component are the same schema, implemented as their dependencies arrive.

## Deferred

- Drag-and-drop authoring. The requirement is the page document and a renderer. A visual editor can sit on top of a document that already works.
- Customer-portal authentication. A portal page is a page plus an external actor. The actor model belongs to [permissions](../../kernel/permissions/README.md).

## Related

- [UI runtime](../ui-runtime/README.md)
- [Metadata](../../kernel/metadata/README.md)
- [Reporting](../reporting/README.md)
- [Primitives](../../primitives/README.md#page)
