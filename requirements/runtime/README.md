# Runtime

The runtime turns metadata into an application a person can use. Renderers draw records. Pages compose screens. Workflows react to events. Search, files, and reporting are product capabilities on the same layer, scheduled after the spine.

The runtime is a client of the [kernel](../kernel/README.md). It does not grow its own tables for Account, Contact, or Opportunity.

| ID | Requirement | Phase | Role |
| --- | --- | --- | --- |
| R5 | [UI runtime](ui-runtime/README.md) | 2 | Generic renderers. Metadata in, React out. |
| R6 | [Page builder](page-builder/README.md) | 2 | Composed screens: record pages first, then dashboards and portals. |
| R8 | [Automation](automation/README.md) | 4 | Triggers, conditions, and actions on the event stream. |
| R14 | [Search](search/README.md) | Later | Find records across objects. Postgres full-text first. |
| R15 | [Files](files/README.md) | Later | Attachments, versions, and previews on records. |
| R16 | [Reporting](reporting/README.md) | Later | Aggregations, dashboards, and eventually an analytics store. |

The layer diagram in the [vision](../vision/README.md) also names activities, notifications, and approvals. Activities are objects (Task, Event). Notifications are jobs fired by workflows. Approvals are workflows that wait on a person and then branch. Those three are covered here and in [jobs](../kernel/jobs/README.md), and they do not have their own requirements.

## Related

- [Kernel](../kernel/README.md)
- [Roadmap](../roadmap/README.md)
- [Requirements index](../README.md)
