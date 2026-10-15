import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { api } from "./api.js"
import { objectTablePath } from "./routes.js"
import { Actions, Button, Field, List, ListButton, Meta, Notice, Page, Panel } from "./ui.jsx"

const fieldTypes = ["text", "number", "currency", "date", "boolean", "enum"]

export function Objects({ token }) {
  const navigate = useNavigate()
  const [objects, setObjects] = useState([])
  const [error, setError] = useState("")
  const [name, setName] = useState("")
  const [label, setLabel] = useState("")
  const [fields, setFields] = useState([{ name: "name", type: "text", required: true, options: "" }])

  async function reload() {
    const body = await api(token, "GET", "/api/meta/objects")
    setObjects(body.objects)
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
        fields: fields.filter((field) => field.name).map((field) => ({
          name: field.name,
          type: field.type,
          required: field.required,
          options: field.type === "enum" ? field.options.split(",").map((item) => item.trim()).filter(Boolean) : undefined,
        })),
      })
      setName("")
      setLabel("")
      setFields([{ name: "name", type: "text", required: true, options: "" }])
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
          <ListButton key={object.id} onClick={() => navigate(objectTablePath(object.name))}>
            <strong>{object.label}</strong>
            <Meta>{object.name}</Meta>
          </ListButton>
        ))}
      </List>
      <Panel title="New object" onSubmit={createObject}>
        <Field label="Name"><input value={name} onChange={(event) => setName(event.target.value)} required /></Field>
        <Field label="Label"><input value={label} onChange={(event) => setLabel(event.target.value)} required /></Field>
        {fields.map((field, index) => (
          <div className="row g-2 align-items-center mb-2" key={index}>
            <div className="col-12 col-md">
              <input className="form-control" value={field.name} aria-label="Field name" onChange={(event) => updateField(index, { name: event.target.value })} />
            </div>
            <div className="col-12 col-md-3">
              <select className="form-select" value={field.type} aria-label="Field type" onChange={(event) => updateField(index, { type: event.target.value })}>
                {fieldTypes.map((type) => <option key={type}>{type}</option>)}
              </select>
            </div>
            <div className="col-auto">
              <label className="form-check mb-0">
                <input className="form-check-input" type="checkbox" checked={field.required} onChange={(event) => updateField(index, { required: event.target.checked })} />
                <span className="form-check-label">Required</span>
              </label>
            </div>
            {field.type === "enum" && (
              <div className="col-12">
                <input className="form-control" value={field.options} placeholder="options, comma separated" onChange={(event) => updateField(index, { options: event.target.value })} />
              </div>
            )}
          </div>
        ))}
        <Actions>
          <Button type="button" variant="secondary" onClick={() => setFields([...fields, { name: "", type: "text", required: false, options: "" }])}>Add field</Button>
          <Button>Create object</Button>
        </Actions>
      </Panel>
    </Page>
  )
}
