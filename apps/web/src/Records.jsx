import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { api } from "./api.js"
import { objectRecordPath, objectTablePath } from "./routes.js"
import { Actions, Button, Field, List, ListButton, Meta, Notice, Page, Panel } from "./ui.jsx"

export function TablePage({ token, object }) {
  const navigate = useNavigate()
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
    <Page title={object} actions={<Button type="button" onClick={() => navigate(objectRecordPath(object))}>New record</Button>}>
      <Notice tone="error">{error}</Notice>
      <form className="d-flex flex-wrap gap-2 align-items-center mb-3" onSubmit={applyFilter}>
        <input className="form-control of-filter" value={field} onChange={(event) => setField(event.target.value)} aria-label="Filter field" />
        <select className="form-select of-filter" value={operator} onChange={(event) => setOperator(event.target.value)} aria-label="Filter operator">
          <option value="contains">contains</option>
          <option value="eq">equals</option>
          <option value="gte">at least</option>
        </select>
        <input className="form-control of-filter" value={value} onChange={(event) => setValue(event.target.value)} aria-label="Filter value" />
        <Button>Filter</Button>
      </form>
      <div className="table-responsive card">
        <table className="table table-hover mb-0">
          <thead>
            <tr>{columns.map((column) => <th key={column}>{column}</th>)}</tr>
          </thead>
          <tbody>
            {records.map((record) => (
              <tr key={record.id} onClick={() => navigate(objectRecordPath(object, record.id))}>
                {columns.map((column) => <td key={column}>{show(record.data[column])}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Notice>{records.length === 0 ? "No records." : ""}</Notice>
    </Page>
  )
}

export function RecordPage({ token, object, id }) {
  const navigate = useNavigate()
  const [fields, setFields] = useState([])
  const [layout, setLayout] = useState([])
  const [relationships, setRelationships] = useState([])
  const [related, setRelated] = useState([])
  const [data, setData] = useState({})
  const [version, setVersion] = useState(0)
  const [entries, setEntries] = useState([])
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
        setEntries([])
        return
      }
      const [record, groups, audit] = await Promise.all([
        api(token, "GET", `/api/objects/${object}/${id}`),
        Promise.all(links.relationships.map(async (link) => {
          const body = await api(token, "GET", `/api/objects/${object}/${id}/related/${link.name}`)
          return { ...link, records: body.records }
        })),
        api(token, "GET", `/api/objects/${object}/${id}/audit`),
      ])
      if (!live) return
      setData(record.data)
      setVersion(record.version)
      setRelated(groups)
      setEntries(audit.entries || [])
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
      navigate(objectRecordPath(object, saved.id))
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
      navigate(objectTablePath(object))
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <Page
      title={data.name || (id ? object : `New ${object}`)}
      actions={<Button type="button" variant="secondary" onClick={() => navigate(objectTablePath(object))}>Back</Button>}
    >
      <Notice tone="error">{error}</Notice>
      <Panel onSubmit={save}>
        {layout.map((name) => {
          const field = byName[name]
          if (!field) return null
          const link = relationships.find((item) => item.name === name && data[name])
          return (
            <div key={name}>
              <Field label={`${field.name}${field.required ? " *" : ""}`}>
                <FieldInput field={field} value={data[name]} onChange={(value) => setValue(name, value)} />
              </Field>
              {link && (
                <div className="mb-3">
                  <Button type="button" variant="text" onClick={() => navigate(objectRecordPath(link.to, data[name]))}>
                    Open {link.to}
                  </Button>
                </div>
              )}
            </div>
          )
        })}
        <Actions>
          <Button>{id ? "Save" : "Create"}</Button>
          {id && <Button type="button" variant="secondary" onClick={remove}>Delete</Button>}
          {id && <span className="text-secondary">Version {version}</span>}
        </Actions>
      </Panel>
      {related.map((group) => (
        <Panel key={group.name} title={group.name}>
          <List>
            {group.records.map((record) => (
              <ListButton key={record.id} onClick={() => navigate(objectRecordPath(group.to, record.id))}>
                <strong>{record.data.name || record.id}</strong>
                <Meta>{group.to}</Meta>
              </ListButton>
            ))}
          </List>
          <Notice>{group.records.length === 0 ? "None yet." : ""}</Notice>
        </Panel>
      ))}
      {id && (
        <Panel title="Activity">
          {entries.length === 0 && <Notice>No activity yet.</Notice>}
          <List>
            {entries.map((entry) => (
              <div key={entry.id} className="list-group-item">
                <div className="fw-semibold">{entry.name}</div>
                {(entry.field != null || entry.before != null || entry.after != null) && (
                  <Meta>{entry.field != null ? `${entry.field}: ` : ""}{auditValue(entry.before)} → {auditValue(entry.after)}</Meta>
                )}
                <Meta>{entry.source}</Meta>
                <Meta>{entry.actor_id}</Meta>
                <Meta>{entry.event_id}</Meta>
                <Meta>{entry.created_at}</Meta>
              </div>
            ))}
          </List>
        </Panel>
      )}
    </Page>
  )
}

function FieldInput({ field, value, onChange }) {
  if (field.type === "boolean") {
    return <input className="form-check-input" type="checkbox" checked={Boolean(value)} onChange={(event) => onChange(event.target.checked)} />
  }
  if (field.type === "enum") {
    return (
      <select className="form-select" value={value ?? ""} onChange={(event) => onChange(event.target.value)}>
        <option value=""></option>
        {(field.options || []).map((option) => <option key={option}>{option}</option>)}
      </select>
    )
  }
  if (field.type === "number" || field.type === "currency") {
    return (
      <input
        className="form-control"
        type="number"
        value={value ?? ""}
        onChange={(event) => onChange(event.target.value === "" ? null : Number(event.target.value))}
      />
    )
  }
  const type = field.type === "date" ? "date" : "text"
  return <input className="form-control" type={type} value={value ?? ""} onChange={(event) => onChange(event.target.value)} />
}

function show(value) {
  if (value === true) return "yes"
  if (value === false) return "no"
  if (value === undefined || value === null || value === "") return ""
  return String(value)
}

function auditValue(value) {
  if (value !== null && typeof value === "object") return JSON.stringify(value)
  return show(value)
}
