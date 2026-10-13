# Files

**R15 · Application runtime · Later**

Depends on the [object model](../../kernel/object-model/README.md) for the record a file hangs from, and on [permissions](../../kernel/permissions/README.md) for who may see it. Bytes live in object storage.

Files are attachments and documents related to records. They are specified now because records need a place to point, and they are built when a milestone actually needs them. Docmost is the reference for a document and knowledge experience. The storage foundation is S3-compatible object storage.

## Why it exists

A company record without its contract, proposal, or logo is an incomplete system of record. Files have their own permissions, versions, and previews, and they still belong to the same organization and the same audit story as the record they hang from.

## Model

```text
Object storage
Metadata
Permissions
Versions
Previews
Attachments
Links
Document relationships
```

```text
Account
 ├── contract.pdf
 ├── proposal.pdf
 └── logo.png
```

The file's metadata (name, type, size, version, which record it is attached to) is platform data. The bytes are in object storage. A link can point at a file without copying it. Document relationships connect a file to one or more records.

Access follows the record's permission by default, and a file can carry a stricter rule. A hidden field's discipline applies here too: a file the actor may not see is absent from the record page.

## Capabilities

- Attach a file to a record, and list a record's files.
- Store bytes in S3-compatible object storage, with metadata in Postgres.
- Keep versions of a file.
- Generate a preview where the type allows it.
- Apply permissions, including a rule stricter than the parent record.
- Relate one file to several records without duplicating the bytes.
- Audit upload, replace, and delete with the source channels from [audit](../../kernel/audit/README.md).

## Decisions

- S3-compatible object storage is the foundation.
- File metadata is platform data. Object storage is not the system of record for who can see a file.
- The requirement waits until the spine needs it. The record model should still be able to gain an attachment without a breaking change.

## Deferred

- Collaborative editing and a full knowledge base. Docmost is the experience to study if that becomes a goal. Phase scope for this requirement is attach, version, preview, and permit.
- Virus scanning and content indexing. Indexing file contents for [search](../search/README.md) follows once both exist.

## Related

- [Object model](../../kernel/object-model/README.md)
- [Permissions](../../kernel/permissions/README.md)
- [Search](../search/README.md)
- [Audit](../../kernel/audit/README.md)
