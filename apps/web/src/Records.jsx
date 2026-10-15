import { useEffect, useState } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { api } from "./api.js"
import { boardColumns, boardMove, cardColumn, cardTitle, cardsInColumn, viewNameOk } from "./board.js"
import { calendarMove, cardsOnDay, monthLabel, monthWeeks, recordDay, shiftMonth, unscheduledCards, weekdays } from "./calendar.js"
import { barIndexes, monthDays, recordSpan, shiftSpan, timelineMove } from "./timeline.js"
import { dedupeRelationships, isChildList, isLinkList, safeReturnPath } from "./relate.js"
import { objectFieldsPath, objectRecordPath, objectTablePath } from "./routes.js"
import { Actions, Button, Field, List, ListButton, Meta, Notice, Page, Panel } from "./ui.jsx"

export function TablePage({ token, object }) {
  const navigate = useNavigate()
  const [scope, setScope] = useState(object)
  const [views, setViews] = useState([])
  const [viewName, setViewName] = useState("all")
  const [fields, setFields] = useState([])
  const [columns, setColumns] = useState([])
  const [records, setRecords] = useState([])
  const [error, setError] = useState("")
  const [field, setField] = useState("name")
  const [operator, setOperator] = useState("contains")
  const [value, setValue] = useState("")
  const [query, setQuery] = useState("")
  const [refresh, setRefresh] = useState(0)
  const [ready, setReady] = useState(false)
  const [boardName, setBoardName] = useState("")
  const [boardLabel, setBoardLabel] = useState("")
  const [boardGroup, setBoardGroup] = useState("")
  const [boardEnd, setBoardEnd] = useState("")
  const [viewKind, setViewKind] = useState("kanban")

  if (scope !== object) {
    setScope(object)
    setViewName("all")
    setQuery("")
    setFields([])
    setViews([])
    setRecords([])
    setReady(false)
    setBoardName("")
    setBoardLabel("")
    setBoardGroup("")
    setBoardEnd("")
    setViewKind("kanban")
    setError("")
  }

  useEffect(() => {
    let live = true
    async function load() {
      const [definition, listed] = await Promise.all([
        api(token, "GET", `/api/meta/objects/${object}`),
        api(token, "GET", `/api/meta/objects/${object}/views`),
      ])
      const available = listed.views || []
      const chosen = available.find((item) => item.name === viewName) || available.find((item) => item.name === "all") || available[0]
      const name = chosen?.name || "all"
      const params = new URLSearchParams(query)
      params.set("view", name)
      if (chosen?.kind === "kanban" || chosen?.kind === "calendar" || chosen?.kind === "timeline") params.set("limit", "200")
      const body = await api(token, "GET", `/api/objects/${object}?${params}`)
      if (!live) return
      setError("")
      setFields(definition.fields || [])
      setViews(available)
      setColumns(chosen?.resolved || [])
      setRecords(body.records || [])
      setReady(true)
      if (name !== viewName) setViewName(name)
    }
    load().catch((err) => { if (live) setError(err.message) })
    return () => { live = false }
  }, [token, object, viewName, query, refresh])

  function applyFilter(event) {
    event.preventDefault()
    setError("")
    if (value === "") {
      setQuery("")
      return
    }
    const params = new URLSearchParams()
    params.append("filter", `${field}:${operator}:${value}`)
    setQuery(params.toString())
  }

  const current = views.find((item) => item.name === viewName)
  const enums = fields.filter((item) => item.type === "enum")
  const dates = fields.filter((item) => item.type === "date")
  const groupFields = viewKind === "calendar" || viewKind === "timeline" ? dates : enums
  const startValue = dates.some((item) => item.name === boardGroup) ? boardGroup : (dates[0]?.name || "")
  const endValue = dates.some((item) => item.name === boardEnd && item.name !== startValue)
    ? boardEnd
    : (dates.find((item) => item.name !== startValue)?.name || "")
  const groupValue = viewKind === "timeline"
    ? startValue
    : (groupFields.some((item) => item.name === boardGroup) ? boardGroup : (groupFields[0]?.name || ""))
  const groupField = fields.find((item) => item.name === current?.group)
  const viewPlaceholder = viewKind === "timeline" ? "schedule" : viewKind === "calendar" ? "renewals" : "pipeline"
  const labelPlaceholder = viewKind === "timeline" ? "Schedule" : viewKind === "calendar" ? "Renewals" : "Pipeline"

  async function createView(event) {
    event.preventDefault()
    setError("")
    const name = boardName.trim()
    if (!viewNameOk(name)) {
      setError("invalid name")
      return
    }
    if (viewKind === "timeline" && (!endValue || endValue === startValue)) {
      setError("end must be a different date")
      return
    }
    const body = {
      name,
      label: boardLabel.trim(),
      kind: viewKind,
      group: groupValue,
      columns: ["*"],
    }
    if (viewKind === "timeline") body.end = endValue
    try {
      await api(token, "POST", `/api/meta/objects/${object}/views`, body)
      setBoardName("")
      setBoardLabel("")
      setViewName(name)
      setRefresh((item) => item + 1)
    } catch (err) {
      setError(err.message)
    }
  }

  async function moveRecord(record, payload, same) {
    if (!current?.group || same) return
    setError("")
    try {
      await api(token, "PATCH", `/api/objects/${object}/${record.id}`, {
        version: record.version,
        data: payload,
      })
      setRefresh((item) => item + 1)
    } catch (err) {
      setError(err.message)
    }
  }

  function moveCard(record, column) {
    moveRecord(
      record,
      boardMove(current?.group, column),
      cardColumn(record, current?.group, groupField?.options || []) === column,
    )
  }

  function moveDay(record, day) {
    moveRecord(record, calendarMove(current?.group, day), recordDay(record, current?.group) === day)
  }

  function moveSpan(record, start, end) {
    moveRecord(
      record,
      timelineMove(current?.group, current?.end, start, end),
      recordDay(record, current?.group) === (start || "") && recordDay(record, current?.end) === (end || ""),
    )
  }

  return (
    <Page
      title={object}
      actions={(
        <>
          <Button type="button" variant="secondary" onClick={() => navigate(objectFieldsPath(object))}>Fields</Button>
          <Button type="button" onClick={() => navigate(objectRecordPath(object))}>New record</Button>
        </>
      )}
    >
      <Notice tone="error">{error}</Notice>
      <form className="d-flex flex-wrap gap-2 align-items-center mb-3" onSubmit={applyFilter}>
        <select className="form-select of-filter" value={viewName} aria-label="View" onChange={(event) => setViewName(event.target.value)}>
          {views.map((item) => <option key={item.name} value={item.name}>{item.label}</option>)}
        </select>
        <input className="form-control of-filter" value={field} onChange={(event) => setField(event.target.value)} aria-label="Filter field" />
        <select className="form-select of-filter" value={operator} onChange={(event) => setOperator(event.target.value)} aria-label="Filter operator">
          <option value="contains">contains</option>
          <option value="eq">equals</option>
          <option value="gte">at least</option>
        </select>
        <input className="form-control of-filter" value={value} onChange={(event) => setValue(event.target.value)} aria-label="Filter value" />
        <Button>Filter</Button>
      </form>
      {ready && enums.length === 0 && dates.length === 0 ? (
        <Notice>Add an enum field to group a board, a date field for a calendar, or two date fields for a timeline.</Notice>
      ) : ready ? (
        <form className="d-flex flex-wrap gap-2 align-items-center mb-3" onSubmit={createView}>
          <select className="form-select of-filter" value={viewKind} aria-label="View kind" onChange={(event) => setViewKind(event.target.value)}>
            <option value="kanban">Board</option>
            <option value="calendar">Calendar</option>
            <option value="timeline">Timeline</option>
          </select>
          {viewKind === "timeline" && dates.length < 2 ? (
            <Notice>Add two date fields to draw a timeline.</Notice>
          ) : groupFields.length === 0 ? (
            <Notice>{viewKind === "calendar" ? "Add a date field to group a calendar." : "Add an enum field to group a board."}</Notice>
          ) : (
            <>
              <input className="form-control of-filter" value={boardName} onChange={(event) => setBoardName(event.target.value)} aria-label="View name" placeholder={viewPlaceholder} required />
              <input className="form-control of-filter" value={boardLabel} onChange={(event) => setBoardLabel(event.target.value)} aria-label="View label" placeholder={labelPlaceholder} required />
              {viewKind === "timeline" ? (
                <>
                  <select className="form-select of-filter" value={startValue} aria-label="Start field" onChange={(event) => setBoardGroup(event.target.value)}>
                    {dates.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
                  </select>
                  <select className="form-select of-filter" value={endValue} aria-label="End field" onChange={(event) => setBoardEnd(event.target.value)}>
                    {dates.filter((item) => item.name !== startValue).map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
                  </select>
                </>
              ) : (
                <select className="form-select of-filter" value={groupValue} aria-label="Group field" onChange={(event) => setBoardGroup(event.target.value)}>
                  {groupFields.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
                </select>
              )}
              <Button>{viewKind === "timeline" ? "Add timeline" : viewKind === "calendar" ? "Add calendar" : "Add board"}</Button>
            </>
          )}
        </form>
      ) : null}
      {current?.kind === "kanban" ? (
        <Board
          group={current.group}
          options={groupField?.options || []}
          records={records}
          onOpen={(id) => navigate(objectRecordPath(object, id))}
          onMove={moveCard}
        />
      ) : current?.kind === "calendar" ? (
        <Month
          group={current.group}
          records={records}
          onOpen={(id) => navigate(objectRecordPath(object, id))}
          onMove={moveDay}
        />
      ) : current?.kind === "timeline" ? (
        <Timeline
          group={current.group}
          end={current.end}
          records={records}
          onOpen={(id) => navigate(objectRecordPath(object, id))}
          onMove={moveSpan}
        />
      ) : (
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
      )}
      <Notice>{records.length === 0 ? "No records." : ""}</Notice>
    </Page>
  )
}

function Board({ group, options, records, onOpen, onMove }) {
  const lanes = boardColumns(options, records, group)
  function dropOn(event, column) {
    event.preventDefault()
    const id = event.dataTransfer.getData("text/plain")
    const record = records.find((item) => item.id === id)
    if (record) onMove(record, column)
  }
  return (
    <div className="of-board">
      {lanes.map((lane) => (
        <section
          key={lane.key || "none"}
          className="of-lane"
          onDragOver={(event) => event.preventDefault()}
          onDrop={(event) => dropOn(event, lane.key)}
        >
          <h2 className="h6">{lane.label}</h2>
          {cardsInColumn(records, group, lane.key, options).map((record) => (
            <article
              key={record.id}
              className="card of-card"
              draggable
              onDragStart={(event) => event.dataTransfer.setData("text/plain", record.id)}
            >
              <button type="button" className="btn btn-link p-0" onClick={() => onOpen(record.id)}>
                {cardTitle(record)}
              </button>
              <select
                className="form-select form-select-sm mt-2"
                aria-label={`Move ${cardTitle(record)}`}
                value={cardColumn(record, group, options)}
                onChange={(event) => onMove(record, event.target.value)}
              >
                {lanes.map((option) => (
                  <option key={option.key || "none"} value={option.key}>{option.label}</option>
                ))}
              </select>
            </article>
          ))}
        </section>
      ))}
    </div>
  )
}

function Month({ group, records, onOpen, onMove }) {
  const now = new Date()
  const [cursor, setCursor] = useState({ year: now.getFullYear(), month: now.getMonth() + 1 })
  const weeks = monthWeeks(cursor.year, cursor.month)
  const loose = unscheduledCards(records, group)
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`

  function dropOn(event, day) {
    event.preventDefault()
    const id = event.dataTransfer.getData("text/plain")
    const record = records.find((item) => item.id === id)
    if (record) onMove(record, day)
  }

  return (
    <div className="of-calendar">
      <div className="d-flex justify-content-between align-items-center gap-2 mb-2">
        <Button type="button" variant="secondary" onClick={() => setCursor((item) => shiftMonth(item.year, item.month, -1))}>Previous</Button>
        <strong>{monthLabel(cursor.year, cursor.month)}</strong>
        <Button type="button" variant="secondary" onClick={() => setCursor((item) => shiftMonth(item.year, item.month, 1))}>Next</Button>
      </div>
      {loose.length > 0 && (
        <section className="of-unscheduled mb-3" onDragOver={(event) => event.preventDefault()} onDrop={(event) => dropOn(event, "")}>
          <h2 className="h6">Unscheduled</h2>
          <div className="d-flex flex-wrap gap-2">
            {loose.map((record) => (
              <CalendarCard key={record.id} record={record} group={group} onOpen={onOpen} onMove={onMove} />
            ))}
          </div>
        </section>
      )}
      <div className="of-month">
        {weekdays.map((day) => <div key={day} className="of-weekday">{day}</div>)}
        {weeks.flat().map((day) => (
          <section
            key={day.date}
            className={`of-day${day.inMonth ? "" : " outside"}${day.date === today ? " today" : ""}`}
            onDragOver={(event) => event.preventDefault()}
            onDrop={(event) => dropOn(event, day.date)}
          >
            <div className="of-day-label">{Number(day.date.slice(8))}</div>
            {cardsOnDay(records, group, day.date).map((record) => (
              <CalendarCard key={record.id} record={record} group={group} onOpen={onOpen} onMove={onMove} />
            ))}
          </section>
        ))}
      </div>
    </div>
  )
}

function Timeline({ group, end, records, onOpen, onMove }) {
  const now = new Date()
  const [cursor, setCursor] = useState({ year: now.getFullYear(), month: now.getMonth() + 1 })
  const days = monthDays(cursor.year, cursor.month)
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`
  const template = { gridTemplateColumns: `14rem repeat(${days.length}, minmax(1.6rem, 1fr))` }
  const loose = records.filter((record) => recordSpan(record, group, end).start === "")
  const placed = records.filter((record) => recordSpan(record, group, end).start !== "")

  function dropOn(event, day) {
    event.preventDefault()
    const id = event.dataTransfer.getData("text/plain")
    const record = records.find((item) => item.id === id)
    if (!record) return
    const span = recordSpan(record, group, end)
    const next = shiftSpan(span.start, span.end, day)
    onMove(record, next.start, next.end)
  }

  return (
    <div className="of-timeline">
      <div className="d-flex justify-content-between align-items-center gap-2 mb-2">
        <Button type="button" variant="secondary" onClick={() => setCursor((item) => shiftMonth(item.year, item.month, -1))}>Previous</Button>
        <strong>{monthLabel(cursor.year, cursor.month)}</strong>
        <Button type="button" variant="secondary" onClick={() => setCursor((item) => shiftMonth(item.year, item.month, 1))}>Next</Button>
      </div>
      {loose.length > 0 && (
        <section className="of-unscheduled mb-3" onDragOver={(event) => event.preventDefault()} onDrop={(event) => dropOn(event, "")}>
          <h2 className="h6">Unscheduled</h2>
          {loose.map((record) => (
            <TimelineLabel key={record.id} record={record} group={group} end={end} onOpen={onOpen} onMove={onMove} />
          ))}
        </section>
      )}
      <div className="of-time-row of-time-head" style={template}>
        <div className="of-time-label" />
        {days.map((day) => (
          <div
            key={day}
            className={`of-time-cell${day === today ? " today" : ""}`}
            onDragOver={(event) => event.preventDefault()}
            onDrop={(event) => dropOn(event, day)}
          >
            {Number(day.slice(8))}
          </div>
        ))}
      </div>
      {placed.map((record) => {
        const span = recordSpan(record, group, end)
        const place = barIndexes(span.start, span.end, days)
        return (
          <div key={record.id} className="of-time-row" style={template}>
            <TimelineLabel record={record} group={group} end={end} onOpen={onOpen} onMove={onMove} />
            {days.map((day) => (
              <div
                key={day}
                className={`of-time-cell${day === today ? " today" : ""}`}
                onDragOver={(event) => event.preventDefault()}
                onDrop={(event) => dropOn(event, day)}
              />
            ))}
            {place && (
              <div
                className="of-time-bar"
                draggable
                title={cardTitle(record)}
                style={{ gridColumn: `${place.from + 2} / ${place.to + 3}` }}
                onDragStart={(event) => event.dataTransfer.setData("text/plain", record.id)}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

function TimelineLabel({ record, group, end, onOpen, onMove }) {
  const span = recordSpan(record, group, end)
  return (
    <div className="of-time-label">
      <button type="button" className="btn btn-link p-0" onClick={() => onOpen(record.id)}>{cardTitle(record)}</button>
      <div className="d-flex gap-1 mt-1">
        <input
          className="form-control form-control-sm"
          type="date"
          aria-label={`Start ${cardTitle(record)}`}
          value={span.start}
          onChange={(event) => onMove(record, event.target.value, span.end)}
        />
        <input
          className="form-control form-control-sm"
          type="date"
          aria-label={`End ${cardTitle(record)}`}
          value={span.end}
          onChange={(event) => onMove(record, span.start, event.target.value)}
        />
      </div>
    </div>
  )
}

function CalendarCard({ record, group, onOpen, onMove }) {
  return (
    <article
      className="card of-card"
      draggable
      onDragStart={(event) => event.dataTransfer.setData("text/plain", record.id)}
    >
      <button type="button" className="btn btn-link p-0" onClick={() => onOpen(record.id)}>
        {cardTitle(record)}
      </button>
      <input
        className="form-control form-control-sm mt-2"
        type="date"
        aria-label={`Move ${cardTitle(record)}`}
        value={recordDay(record, group)}
        onChange={(event) => onMove(record, event.target.value)}
      />
    </article>
  )
}

export function RecordPage({ token, object, id }) {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const search = searchParams.toString()
  const [fields, setFields] = useState([])
  const [layout, setLayout] = useState([])
  const [relationships, setRelationships] = useState([])
  const [related, setRelated] = useState([])
  const [data, setData] = useState({})
  const [version, setVersion] = useState(0)
  const [entries, setEntries] = useState([])
  const [error, setError] = useState("")
  const [refresh, setRefresh] = useState(0)

  useEffect(() => {
    let live = true
    async function load() {
      const [definition, form, links] = await Promise.all([
        api(token, "GET", `/api/meta/objects/${object}`),
        api(token, "GET", `/api/meta/objects/${object}/layouts/record`),
        api(token, "GET", `/api/meta/objects/${object}/relationships`),
      ])
      if (!live) return
      const unique = dedupeRelationships(links.relationships)
      setFields(definition.fields)
      setLayout(form.resolved)
      setRelationships(unique)
      if (!id) {
        const params = new URLSearchParams(search)
        const seeded = {}
        for (const field of definition.fields) {
          const value = params.get(field.name)
          if (value) seeded[field.name] = value
        }
        setData(seeded)
        setVersion(0)
        setRelated([])
        setEntries([])
        return
      }
      const [record, groups, audit] = await Promise.all([
        api(token, "GET", `/api/objects/${object}/${id}`),
        Promise.all(unique.map(async (link) => {
          const body = await api(token, "GET", `/api/objects/${object}/${id}/related/${link.name}`)
          return { ...link, records: body.records || [] }
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
  }, [token, object, id, search, refresh])

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
      const back = safeReturnPath(new URLSearchParams(search).get("back"))
      navigate(id || !back ? objectRecordPath(object, saved.id) : back)
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
        <RelatedPanel
          key={group.name}
          token={token}
          object={object}
          id={id}
          group={group}
          fields={fields}
          onLinked={() => setRefresh((value) => value + 1)}
        />
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

function RelatedPanel({ token, object, id, group, fields, onLinked }) {
  const navigate = useNavigate()
  const [draft, setDraft] = useState("")
  const [error, setError] = useState("")
  const child = isChildList(group, fields)
  const link = isLinkList(group)

  async function linkRecord(event) {
    event.preventDefault()
    setError("")
    try {
      await api(token, "POST", `/api/objects/${object}/${id}/related/${group.name}`, { id: draft.trim() })
      setDraft("")
      onLinked()
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <Panel title={group.name}>
      <Notice tone="error">{error}</Notice>
      {child && (
        <div className="mb-3">
          <Button
            type="button"
            onClick={() => navigate(objectRecordPath(group.to, undefined, {
              [group.inverse]: id,
              back: objectRecordPath(object, id),
            }))}
          >
            Add {group.to}
          </Button>
        </div>
      )}
      {link && (
        <form className="d-flex flex-wrap gap-2 align-items-center mb-3" onSubmit={linkRecord}>
          <input className="form-control of-filter" value={draft} onChange={(event) => setDraft(event.target.value)} aria-label={`Id for ${group.name}`} placeholder="Existing record id" required />
          <Button>Link</Button>
        </form>
      )}
      <List>
        {(group.records || []).map((record) => (
          <ListButton key={record.id} onClick={() => navigate(objectRecordPath(group.to, record.id))}>
            <strong>{record.data?.name || record.id}</strong>
            <Meta>{group.to}</Meta>
          </ListButton>
        ))}
      </List>
      <Notice>{(group.records || []).length === 0 ? "None yet." : ""}</Notice>
    </Panel>
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
