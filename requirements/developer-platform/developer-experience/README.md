# Developer experience

**R17 · Developer platform · Phase 6**

Depends on [metadata](../../kernel/metadata/README.md) and on [environments](../environments/README.md). Functions in the project layout are specified by [functions](../functions/README.md). The HTTP surface is the [public API](../../integrations/webhooks/README.md).

A developer should be able to treat a business application the way they treat infrastructure. The description of the application is files. Git is the source of truth. A CLI moves those files between a local environment and a deployed one.

The original notes used `acme` as a placeholder for the CLI. The command in this spec is `openforce`.

## Why it exists

Metadata that can only be clicked into a production UI will drift, and nobody will be able to review a change to a workflow the way they review a change to code. Salesforce has historically made this awkward. An application whose objects, layouts, workflows, and permissions are files is one of the ways Openforce can be meaningfully better.

Putting this high on the roadmap is deliberate. An object model that cannot round-trip through a file will grow a second, click-only source of truth, and the two will diverge.

## Model

Commands:

```text
openforce init
openforce dev
openforce pull
openforce push
openforce migrate
openforce deploy
```

`init` creates a project. `dev` runs it locally against a development environment. `pull` and `push` move metadata between that environment and the working tree. `migrate` prepares a metadata migration. `deploy` applies a revision to an environment. The environment semantics are [environments](../environments/README.md).

The project:

```text
project/
├── objects/
├── fields/
├── layouts/
├── workflows/
├── pages/
├── functions/
├── integrations/
└── permissions/
```

A declarative object, the kind of file that gets committed:

```yaml
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
```

The concrete syntax (YAML, JSON, or another text format) is open. The requirement is that it is text, diffable, and a complete description of the metadata it covers. The example above is the shape, not a commitment to a parser.

The default CRM ships as a project of this shape, so `examples/crm` in the [intended repository](../../roadmap/README.md#intended-repository-shape) is both the first vertical and the proof that the CLI works.

## Capabilities

- Create a project with the layout above.
- Run that project against a local development environment.
- Export an environment's metadata into the project, and apply the project back, including a diff a person can read.
- Represent objects, fields, layouts, workflows, pages, functions, integrations, and permissions as files.
- Deploy a committed revision to an environment.
- Generate or consume the OpenAPI document from the [public API](../../integrations/webhooks/README.md), so a client can be written against the same metadata.

## Decisions

- Git is the source of truth for an application.
- The CLI is `openforce`.
- The CRM is delivered as a project, through the same commands a tenant's application uses.

## Deferred

- The file format. The shape is specified. The parser is a Phase 6 implementation choice, made early enough that Phase 2 metadata does not become impossible to serialize. Metadata's storage model should stay serializable from the day it exists.
- A hosted editor that writes the same files. Useful, and it is a client of this layout.

## Related

- [Environments](../environments/README.md)
- [Metadata](../../kernel/metadata/README.md)
- [Functions](../functions/README.md)
- [Principles](../../principles/README.md#the-whole-application-is-versioned)
