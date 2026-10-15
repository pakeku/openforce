import { useCallback, useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { api } from "./api.js"
import { fieldDraft, fieldPayload, fieldTypeOptions, linkTargets } from "./fields.js"
import { objectTablePath } from "./routes.js"
import { Actions, Button, Notice, Page, Panel } from "./ui.jsx"

const valuePolicy = "Renaming a field keeps its values. Changing the type or the linked object clears only that field's values. Deleting a field clears them too. The records stay."

export function FieldEditor({ field, objects, onChange, nameRequired = true }) {
  return (
    <div className="row g-2 align-items-center mb-2">
      <div className="col-12 col-md">
        <input className="form-control" value={field.name} aria-label="Field name" required={nameRequired} onChange={(event) => onChange({ name: event.target.value })} />
      </div>
      <div className="col-12 col-md-3">
        <select className="form-select" value={field.type} aria-label="Field type" onChange={(event) => onChange({ type: event.target.value })}>
          {fieldTypeOptions.map((type) => <option key={type} value={type}>{type}</option>)}
        </select>
      </div>
      <div className="col-auto">
        <label className="form-check mb-0">
          <input className="form-check-input" type="checkbox" checked={field.required} onChange={(event) => onChange({ required: event.target.checked })} />
          <span className="form-check-label">Required</span>
        </label>
      </div>
      {field.type === "enum" && (
        <div className="col-12">
          <input className="form-control" value={field.options} aria-label="Choices" placeholder="options, comma separated" onChange={(event) => onChange({ options: event.target.value })} />
        </div>
      )}
      {field.type === "link" && (
        <div className="col-12 col-md-4">
          <select className="form-select" value={field.to} aria-label="Linked object" required onChange={(event) => onChange({ to: event.target.value })}>
            <option value="">Linked object</option>
            {objects.map((object) => <option key={object.name} value={object.name}>{object.label}</option>)}
          </select>
        </div>
      )}
    </div>
  )
}

export function Fields({ token, object }) {
  const navigate = useNavigate()
  const [label, setLabel] = useState(object)
  const [rows, setRows] = useState([])
  const [objects, setObjects] = useState([])
  const [draft, setDraft] = useState(fieldDraft())
  const [error, setError] = useState("")

  const reload = useCallback(async () => {
    const [view, catalog] = await Promise.all([
      api(token, "GET", `/api/meta/objects/${object}`),
      api(token, "GET", "/api/meta/objects"),
    ])
    setLabel(view.label || object)
    setRows((view.fields || []).map((field) => ({ original: field.name, draft: fieldDraft(field) })))
    setObjects(catalog.objects || [])
  }, [token, object])

  useEffect(() => {
    reload().catch((err) => setError(err.message))
  }, [reload])

  function updateRow(index, patch) {
    setRows(rows.map((row, i) => (i === index ? { ...row, draft: { ...row.draft, ...patch } } : row)))
  }

  async function save(index, event) {
    event.preventDefault()
    setError("")
    const row = rows[index]
    try {
      await api(token, "PATCH", `/api/meta/objects/${object}/fields/${encodeURIComponent(row.original)}`, fieldPayload(row.draft))
      await reload()
    } catch (err) {
      setError(err.message)
    }
  }

  async function remove(name) {
    setError("")
    try {
      await api(token, "DELETE", `/api/meta/objects/${object}/fields/${encodeURIComponent(name)}`)
      await reload()
    } catch (err) {
      setError(err.message)
    }
  }

  async function add(event) {
    event.preventDefault()
    setError("")
    try {
      await api(token, "POST", `/api/meta/objects/${object}/fields`, fieldPayload(draft))
      setDraft(fieldDraft())
      await reload()
    } catch (err) {
      setError(err.message)
    }
  }

  const targets = linkTargets(objects, object, label)

  return (
    <Page
      title={`${label} fields`}
      actions={(
        <>
          <Button type="button" variant="secondary" onClick={() => navigate("/objects")}>Objects</Button>
          <Button type="button" variant="secondary" onClick={() => navigate(objectTablePath(object))}>Records</Button>
        </>
      )}
    >
      <Notice>{valuePolicy}</Notice>
      <Notice tone="error">{error}</Notice>
      {rows.map((row, index) => (
        <Panel key={row.original} title={row.original} onSubmit={(event) => save(index, event)}>
          <FieldEditor field={row.draft} objects={targets} onChange={(patch) => updateRow(index, patch)} />
          <Actions>
            <Button>Save</Button>
            <Button type="button" variant="secondary" onClick={() => remove(row.original)}>Delete</Button>
          </Actions>
        </Panel>
      ))}
      {rows.length === 0 && <Notice>No fields yet.</Notice>}
      <Panel title="Add field" onSubmit={add}>
        <FieldEditor field={draft} objects={targets} onChange={(patch) => setDraft({ ...draft, ...patch })} />
        <Button>Add field</Button>
      </Panel>
    </Page>
  )
}
