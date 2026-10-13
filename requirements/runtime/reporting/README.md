# Reporting

**R16 · Application runtime · Later**

Depends on the [query API](../../kernel/query-api/README.md) for operational reads, and on [events](../../kernel/events/README.md) for the later analytical path. Charts are placed on pages by the [page builder](../page-builder/README.md).

Reporting is an application on the data platform. Operational queries and analytical queries are different workloads, and the spec keeps them apart from the start even though both begin on Postgres.

## Why it exists

A list of opportunities is a view. Revenue by stage, a funnel, or a forecast is a report. Salesforce's reporting surface is effectively another application over the same records. Openforce treats it that way: metadata describes the report, and a store answers it. The store starts as Postgres and can move.

## Model

The first path:

```text
Postgres
   ↓
aggregations
   ↓
charts
```

The later path, when operational Postgres should not also serve heavy analytical queries:

```text
Postgres
    ↓
CDC / events
    ↓
Analytics store
```

The surface that path has to grow into:

```text
Reports
Dashboards
Metrics
Funnels
Cohorts
Forecasting
BI
```

A report definition is metadata, versioned with the rest of the application, and it shows up in a field's "used by" list. A dashboard is a [page](../page-builder/README.md) whose components are charts bound to reports.

Phase scope, when this requirement starts, is aggregations and charts on Postgres. Funnels, cohorts, forecasting, and a separate analytics store are the same product, later.

## Capabilities

- Define a report as metadata: an object, aggregations, and filters.
- Run that report on Postgres without disturbing the OLTP path more than a bounded query must.
- Render the result as a chart on a page.
- Respect permissions, so a report cannot reveal records the actor cannot see.
- Subscribe to events when an analytics store exists, so the store is a subscriber and the record write stays a transaction.

## Decisions

- Operational queries and analytical queries are separate concerns.
- The first analytical engine is Postgres.
- A report is metadata. It is not a handwritten SQL file in the CRM.

## Deferred

- The choice of analytics store. The event stream is the feed, whenever that store is chosen.
- Forecasting and cohorts. They need history the audit and event logs will have been collecting. They do not need to exist for the first chart.

## Related

- [Query and API](../../kernel/query-api/README.md)
- [Events](../../kernel/events/README.md)
- [Page builder](../page-builder/README.md)
- [Metadata](../../kernel/metadata/README.md)
