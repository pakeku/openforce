import { useEffect, useState } from "react"
import { api } from "./api.js"

export function TablePage({ token, object, open }) {
  const [columns, setColumns] = useState([])
  const [records, setRecords] = useState([])
  const [error, setError] = useState("")
  const [field, setField] = useState("name")
  const [operator, setOperator] = useState("contains")
  const [value, setValue] = useState("")

  async function load(query) {
    const view = await api(token, "GET", `/api/meta/objects/${object}/views/all`)
    const path = query
      ? `/api/objects/${object}?view=all&${query}`
      : `/api/objects/${object}?view=all`
    const body = await api(token, "GET", path)
    setColumns(view.resolved)
    setRecords(body.records)
  }

  useEffect(() => {
    load("").catch((err) => setError(err.message))
  }, [token, object])

  function applyFilter(event) {
    event.preventDefault()
    setError("")
    const query = value === "" ? "" : `filter=${encodeURIComponent(`${field}:${operator}:${value}`)}`
    load(query).catch((err) => setError(err.message))
  }

  return (
    <main>
      <div className="title-row">
        <h1>{object}</h1>
        <button onClick={() => open({ page: "record", object })}>New record</button>
      </div>
      {error && <p className="error">{error}</p>}
      <form className="filters" onSubmit={applyFilter}>
        <input value={field} onChange={(event) => setField(event.target.value)} aria-label="Filter field" />
        <select value={operator} onChange={(event) => setOperator(event.target.value)} aria-label="Filter operator">
          <option value="contains">contains</option>
          <option value="eq">equals</option>
          <option value="gte">at least</option>
        </select>
        <input value={value} onChange={(event) => setValue(event.target.value)} aria-label="Filter value" />
        <button>Filter</button>
      </form>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>{columns.map((column) => <th key={column}>{column}</th>)}</tr>
          </thead>
          <tbody>
            {records.map((record) => (
              <tr key={record.id} onClick={() => open({ page: "record", object, id: record.id })}>
                {columns.map((column) => <td key={column}>{show(record.data[column])}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {records.length === 0 && <p className="empty">No records.</p>}
    </main>
  )
}

export function RecordPage({ token, object, id, open }) {
  const [fields, setFields] = useState([])
  const [layout, setLayout] = useState([])
  const [relationships, setRelationships] = useState([])
  const [related, setRelated] = useState([])
  const [data, setData] = useState({})
  const [version, setVersion] = useState(0)
  const [error, setError] = useState("")

  useEffect(() => {
    let live = true
    async function load() {
      const [definition, form, links] = await Promise.all([
        api(token, "GET", `/api/meta/objects/${object}`),
        api(token, "GET", `/api/meta/objects/${object}/layouts/record`),
        api(token, "GET", `/api/meta/objects/${object}/relationships`),
      ])
      if (!live) return
      setFields(definition.fields)
      setLayout(form.resolved)
      setRelationships(links.relationships)
      if (!id) {
        setData({})
        setVersion(0)
        setRelated([])
        return
      }
      const record = await api(token, "GET", `/api/objects/${object}/${id}`)
      const groups = await Promise.all(links.relationships.map(async (link) => {
        const body = await api(token, "GET", `/api/objects/${object}/${id}/related/${link.name}`)
        return { ...link, records: body.records }
      }))
      if (!live) return
      setData(record.data)
      setVersion(record.version)
      setRelated(groups)
    }
    load().catch((err) => { if (live) setError(err.message) })
    return () => { live = false }
  }, [token, object, id])

  const byName = Object.fromEntries(fields.map((field) => [field.name, field]))

  function setValue(name, value) {
    setData({ ...data, [name]: value })
  }

  async function save(event) {
    event.preventDefault()
    setError("")
    const payload = {}
    for (const name of layout) {
      if (data[name] !== undefined) payload[name] = data[name]
    }
    try {
      const saved = id
        ? await api(token, "PATCH", `/api/objects/${object}/${id}`, { version, data: payload })
        : await api(token, "POST", `/api/objects/${object}`, payload)
      open({ page: "record", object, id: saved.id })
      setVersion(saved.version)
      setData(saved.data)
    } catch (err) {
      setError(err.message)
    }
  }

  async function remove() {
    setError("")
    try {
      await api(token, "DELETE", `/api/objects/${object}/${id}`)
      open({ page: "table", object })
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <main>
      <div className="title-row">
        <h1>{data.name || (id ? object : `New ${object}`)}</h1>
        <button className="secondary" onClick={() => open({ page: "table", object })}>Back</button>
      </div>
      {error && <p className="error">{error}</p>}
      <form className="panel" onSubmit={save}>
        {layout.map((name) => {
          const field = byName[name]
          if (!field) return null
          const link = relationships.find((item) => item.name === name && data[name])
          return (
            <label key={name}>
              {field.name}{field.required ? " *" : ""}
              <FieldInput field={field} value={data[name]} onChange={(value) => setValue(name, value)} />
              {link && (
                <button type="button" className="text" onClick={() => open({ page: "record", object: link.to, id: data[name] })}>
                  Open {link.to}
                </button>
              )}
            </label>
          )
        })}
        <div className="actions">
          <button>{id ? "Save" : "Create"}</button>
          {id && <button type="button" className="secondary" onClick={remove}>Delete</button>}
          {id && <span className="version">Version {version}</span>}
        </div>
      </form>
      {related.map((group) => (
        <section key={group.name}>
          <h2>{group.name}</h2>
          <ul className="object-list">
            {group.records.map((record) => (
              <li key={record.id}>
                <button onClick={() => open({ page: "record", object: group.to, id: record.id })}>
                  <strong>{record.data.name || record.id}</strong>
                  <span>{group.to}</span>
                </button>
              </li>
            ))}
          </ul>
          {group.records.length === 0 && <p className="empty">None yet.</p>}
        </section>
      ))}
    </main>
  )
}

function FieldInput({ field, value, onChange }) {
  if (field.type === "boolean") {
    return <input type="checkbox" checked={Boolean(value)} onChange={(event) => onChange(event.target.checked)} />
  }
  if (field.type === "enum") {
    return (
      <select value={value ?? ""} onChange={(event) => onChange(event.target.value)}>
        <option value=""></option>
        {(field.options || []).map((option) => <option key={option}>{option}</option>)}
      </select>
    )
  }
  if (field.type === "number" || field.type === "currency") {
    return (
      <input
        type="number"
        value={value ?? ""}
        onChange={(event) => onChange(event.target.value === "" ? null : Number(event.target.value))}
      />
    )
  }
  const type = field.type === "date" ? "date" : "text"
  return <input type={type} value={value ?? ""} onChange={(event) => onChange(event.target.value)} />
}

function show(value) {
  if (value === true) return "yes"
  if (value === false) return "no"
  if (value === undefined || value === null || value === "") return ""
  return String(value)
}
