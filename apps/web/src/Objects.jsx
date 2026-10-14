import { useEffect, useState } from "react"
import { api } from "./api.js"

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
    <main>
      <h1>Objects</h1>
      {error && <p className="error">{error}</p>}
      <ul className="object-list">
        {objects.map((object) => (
          <li key={object.id}>
            <button onClick={() => open({ page: "table", object: object.name })}>
              <strong>{object.label}</strong>
              <span>{object.name}</span>
            </button>
          </li>
        ))}
      </ul>
      <form className="panel" onSubmit={createObject}>
        <h2>New object</h2>
        <label>Name<input value={name} onChange={(event) => setName(event.target.value)} required /></label>
        <label>Label<input value={label} onChange={(event) => setLabel(event.target.value)} required /></label>
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
        <div className="actions">
          <button type="button" className="secondary" onClick={() => setFields([...fields, { name: "", type: "text", required: false, options: "" }])}>Add field</button>
          <button>Create object</button>
        </div>
      </form>
    </main>
  )
}
