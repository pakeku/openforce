# Developer platform

The developer platform is how someone builds a business application on Openforce without editing Openforce. The application lives in git. Environments receive a revision. Custom behavior that a workflow cannot express lives in a function.

This layer is unusually high on the roadmap for a project that also has a CRM to ship. The [phase 6 goal](../roadmap/README.md#phase-6-developer-platform) is the point where the platform is real: a developer can ship an entire application against it.

| ID | Requirement | Phase | Role |
| --- | --- | --- | --- |
| R17 | [Developer experience](developer-experience/README.md) | 6 | The CLI, the project layout, and git as the source of truth. |
| R18 | [Environments](environments/README.md) | 6 | Development, staging, production, and migrations between them. |
| R19 | [Functions](functions/README.md) | 6 | Custom code in a controlled worker. |

The APIs those developers call are specified under [webhooks and the public API](../integrations/webhooks/README.md). The definitions they deploy are [metadata](../kernel/metadata/README.md).

## Related

- [Metadata](../kernel/metadata/README.md)
- [Roadmap](../roadmap/README.md)
- [Requirements index](../README.md)
