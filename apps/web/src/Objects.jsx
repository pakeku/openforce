import { useEffect, useState } from "react"
import { api } from "./api.js"
import { Actions, Button, Field, Notice, Page, Panel } from "./ui.jsx"

const fieldTypes = ["text", "number", "currency", "date", "boolean", "enum"]

export function Objects({ token, open }) {
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
      <ul className="object-list">
        {objects.map((object) => (
          <li key={object.id}>
            <Button onClick={() => open({ page: "table", object: object.name })}>
              <strong>{object.label}</strong>
              <span>{object.name}</span>
            </Button>
          </li>
        ))}
      </ul>
      <Panel title="New object" onSubmit={createObject}>
        <Field label="Name"><input value={name} onChange={(event) => setName(event.target.value)} required /></Field>
        <Field label="Label"><input value={label} onChange={(event) => setLabel(event.target.value)} required /></Field>
        {fields.map((field, index) => (
          <div className="field-row" key={index}>
            <input value={field.name} aria-label="Field name" onChange={(event) => updateField(index, { name: event.target.value })} />
            <select value={field.type} aria-label="Field type" onChange={(event) => updateField(index, { type: event.target.value })}>
              {fieldTypes.map((type) => <option key={type}>{type}</option>)}
            </select>
            <label className="check"><input type="checkbox" checked={field.required} onChange={(event) => updateField(index, { required: event.target.checked })} />Required</label>
            {field.type === "enum" && (
              <input value={field.options} placeholder="options, comma separated" onChange={(event) => updateField(index, { options: event.target.value })} />
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
