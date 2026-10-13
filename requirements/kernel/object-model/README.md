# Object model

**R1 · Platform kernel · Phase 1**

Depends on the [database](../database/README.md). Definitions are stored as [metadata](../metadata/README.md). Records are read and written through the [query API](../query-api/README.md).

The object model is how Openforce represents a business. A tenant adds a new kind of thing by declaring an object and its fields. The platform stores records of that object without a new backend release.

This is the architectural decision the rest of the system sits on.

## Why it exists

Hardcoded tables (`accounts`, `contacts`, `opportunities`) make every new business concept a schema change and a deploy. A definition-driven model makes the CRM, and the next application a tenant invents, the same kind of data.

## Model

Kernel identities exist so tenancy and access have something to attach to:

- **Organization**, the tenant boundary
- **User**
- **Team**

Everything a business tracks is a defined object:

- **ObjectDefinition**, a named kind of record
- **FieldDefinition**, a typed property of that object
- **Record**, one instance, holding values for those fields
- **RecordRelationship**, a link from one record to another

Views and forms are definitions in the same family. Their catalog is specified under [metadata](../metadata/README.md), and they are rendered by the [UI runtime](../../runtime/ui-runtime/README.md).

A standard CRM is a set of object definitions the project ships as configuration:

```text
Account · Contact · Lead · Opportunity · Case · Task · Event
```

The same mechanism defines anything else a tenant needs:

```text
Vendor · Property · Patient · Project · Contract · Vehicle
```

```json
{
  "object": "company",
  "fields": {
    "name": { "type": "text", "required": true },
    "employees": { "type": "number" },
    "industry": {
      "type": "select",
      "options": ["SaaS", "Finance", "Healthcare"]
    }
  }
}
```

Field types are the [field primitive](../../primitives/README.md#field): text, number, currency, date, boolean, enum, relation, and formula. `select` in the example is an enum presented as a closed list of options.

Relationships are `belongs_to`, `has_many`, and `many_to_many`.

## Capabilities

- Declare an object, its fields, and its relationships as data.
- Relate records as `belongs_to`, `has_many`, and `many_to_many`.
- Mark a field required, set its type, and give an enum a fixed set of options.
- Create, read, update, and delete records of any declared object.
- Name organization, user, and team from permissions and audit.
- Ship the CRM objects above as default configuration, next to any object a tenant adds.

## Decisions

- Business objects are configuration. An Account is an object definition, applied through the same path as a tenant-defined object.
- The physical storage of a record is a database concern. The object model does not require one handwritten table per object. See the [database](../database/README.md) decision to start with JSONB documents.

## Working assumptions

- Organization, user, and team are kernel identities, present before a tenant defines any object. The notes list them beside object and field definitions, then treat CRM objects as configuration. This spec keeps that split: identities are platform records, and business objects are metadata.

## Deferred

- The concrete tables, indexes, and tenancy mechanism. Those belong to the database requirement and to the [next design pass](../../roadmap/README.md#next-design-work).
- Formula evaluation. Formula is a field type in the primitive set. The evaluator arrives with the metadata and query work that resolves a formula against a record.

## Related

- [Primitives](../../primitives/README.md)
- [Metadata](../metadata/README.md)
- [Database](../database/README.md)
- [Permissions](../permissions/README.md)
