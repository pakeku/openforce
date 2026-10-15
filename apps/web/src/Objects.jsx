import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { api } from "./api.js"
import { FieldEditor } from "./Fields.jsx"
import { fieldPayload, linkTargets } from "./fields.js"
import { dedupeRelationships, relationshipRequests } from "./relate.js"
import { objectFieldsPath, objectTablePath } from "./routes.js"
import { Actions, Button, Field, List, ListItem, Meta, Notice, Page, Panel } from "./ui.jsx"

const namePattern = /^[a-z][a-z0-9_]*$/

const kindHelp = {
  subobjects: "Any record of the parent can hold many records of the child. Each child stores the parent.",
  one: "Each record points at one record of the other object.",
  links: "Records link to existing records of the other object, in both directions.",
}

export function Objects({ token }) {
  const navigate = useNavigate()
  const [objects, setObjects] = useState([])
  const [error, setError] = useState("")
  const [name, setName] = useState("")
  const [label, setLabel] = useState("")
  const [fields, setFields] = useState([{ name: "name", type: "text", required: true, options: "", to: "" }])
  const [kind, setKind] = useState("subobjects")
  const [from, setFrom] = useState("")
  const [to, setTo] = useState("")
  const [relName, setRelName] = useState("")
  const [inverse, setInverse] = useState("")
  const [fieldName, setFieldName] = useState("")
  const [relationships, setRelationships] = useState([])

  async function reload() {
    const body = await api(token, "GET", "/api/meta/objects")
    const objects = body.objects || []
    setObjects(objects)
    const groups = await Promise.all(objects.map(async (object) => {
      const rel = await api(token, "GET", `/api/meta/objects/${object.name}/relationships`)
      return { object: object.name, label: object.label, items: dedupeRelationships(rel.relationships) }
    }))
    setRelationships(groups.filter((group) => group.items.length > 0))
  }

  useEffect(() => {
    reload().catch((err) => setError(err.message))
  }, [token])

  function updateField(index, patch) {
    setFields(fields.map((field, i) => (i === index ? { ...field, ...patch } : field)))
  }

  async function createObject(event) {
    event.preventDefault()
    setError("")
    try {
      await api(token, "POST", "/api/meta/objects", {
        name,
        label,
        fields: fields.filter((field) => field.name).map((field) => fieldPayload(field)),
      })
      setName("")
      setLabel("")
      setFields([{ name: "name", type: "text", required: true, options: "", to: "" }])
      await reload()
    } catch (err) {
      setError(err.message)
    }
  }

  async function createRelationship(event) {
    event.preventDefault()
    setError("")
    const form = { kind, from, to, name: relName, inverse, field: fieldName }
    const requests = relationshipRequests(form)
    const names = requests.flatMap((request) => [request.body.name, request.body.inverse].filter(Boolean))
    if (!from || !to || !relName.trim() || names.some((item) => !namePattern.test(item))) {
      setError("Use object names and relationship names made of lowercase letters, numbers, and underscores.")
      return
    }
    if (kind === "links" && !inverse.trim()) {
      setError("Links need a name on the other object.")
      return
    }
    try {
      for (const request of requests) {
        try {
          await api(token, "POST", `/api/meta/objects/${request.object}/relationships`, request.body)
        } catch (err) {
          const exists = err.message.includes("already exists")
          if (!(exists && request.body.kind === "belongs_to" && requests.length > 1)) throw err
        }
      }
      setRelName("")
      setInverse("")
      setFieldName("")
      await reload()
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <Page title="Objects">
      <Notice tone="error">{error}</Notice>
      <List>
        {objects.map((object) => (
          <ListItem
            key={object.id}
            title={object.label}
            meta={<Meta>{object.name}</Meta>}
            actions={(
              <Actions>
                <Button type="button" variant="secondary" onClick={() => navigate(objectFieldsPath(object.name))}>Fields</Button>
                <Button type="button" variant="secondary" onClick={() => navigate(objectTablePath(object.name))}>Records</Button>
              </Actions>
            )}
          />
        ))}
      </List>
      <Panel title="New object" onSubmit={createObject}>
        <Field label="Name"><input value={name} onChange={(event) => setName(event.target.value)} required /></Field>
        <Field label="Label"><input value={label} onChange={(event) => setLabel(event.target.value)} required /></Field>
        <Notice>Choose link to point a field at another object. Create that object first, or link this object to itself.</Notice>
        {fields.map((field, index) => (
          <FieldEditor
            key={index}
            field={field}
            objects={linkTargets(objects, name, label)}
            nameRequired={false}
            onChange={(patch) => updateField(index, patch)}
          />
        ))}
        <Actions>
          <Button type="button" variant="secondary" onClick={() => setFields([...fields, { name: "", type: "text", required: false, options: "", to: "" }])}>Add field</Button>
          <Button>Create object</Button>
        </Actions>
      </Panel>
      <details className="card mb-3">
        <summary className="card-header of-disclosure">Child lists and links</summary>
        <form onSubmit={createRelationship}>
          <div className="card-body">
      <Notice>A link to one record is a field on the object. Use this when a record should list many children, or when records link both ways.</Notice>
        <Notice>{kindHelp[kind]}</Notice>
        <Field label="Kind">
          <select value={kind} onChange={(event) => setKind(event.target.value)}>
            <option value="subobjects">Subobjects</option>
            <option value="one">One record</option>
            <option value="links">Links</option>
          </select>
        </Field>
        <Field label={kind === "subobjects" ? "Parent" : "On"}>
          <select value={from} onChange={(event) => setFrom(event.target.value)} required>
            <option value="">Choose</option>
            {objects.map((object) => <option key={object.name} value={object.name}>{object.label}</option>)}
          </select>
        </Field>
        <Field label={kind === "subobjects" ? "Child" : "Other object"}>
          <select value={to} onChange={(event) => setTo(event.target.value)} required>
            <option value="">Choose</option>
            {objects.map((object) => <option key={object.name} value={object.name}>{object.label}</option>)}
          </select>
        </Field>
        <Field label={kind === "subobjects" ? "List name" : kind === "one" ? "Field name" : "Link name"}>
          <input value={relName} onChange={(event) => setRelName(event.target.value)} placeholder={kind === "subobjects" ? "comments" : kind === "links" ? "tags" : "owner"} required />
        </Field>
        {kind === "subobjects" && (
          <Field label="Field on each child">
            <input value={fieldName} onChange={(event) => setFieldName(event.target.value)} placeholder={from || "parent"} />
          </Field>
        )}
        {kind !== "subobjects" && (
          <Field label={kind === "links" ? "Name on the other object" : "List on the other object"}>
            <input value={inverse} onChange={(event) => setInverse(event.target.value)} placeholder={kind === "links" ? "companies" : "optional"} required={kind === "links"} />
          </Field>
        )}
        <Button>Add relationship</Button>
          </div>
        </form>
      </details>
      {relationships.map((group) => (
        <Panel key={group.object} title={group.label}>
          <List>
            {group.items.map((item) => (
              <ListItem
                key={item.name}
                title={item.name}
                meta={<Meta>{item.kind === "has_many" ? "subobjects" : item.kind === "many_to_many" ? "links" : "one record"} · {item.to}</Meta>}
              />
            ))}
          </List>
        </Panel>
      ))}
    </Page>
  )
}
