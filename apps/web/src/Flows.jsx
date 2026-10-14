import { useCallback, useEffect, useState } from "react"
import { Background, Controls, Handle, MiniMap, Position, ReactFlow, applyEdgeChanges, applyNodeChanges } from "@xyflow/react"
import { api } from "./api.js"
import { Button, Notice } from "./ui.jsx"
import { appendElse, appendNode, connectionAllowed, graphToWorkflow, insertBefore, removeNode, workflowToGraph } from "./flow.js"

const events = ["record.created", "record.updated", "record.deleted"]
const operators = ["eq", "neq", "gt", "gte", "lt", "lte", "contains"]
const actionTypes = ["create", "update", "delete", "http", "approval"]

const nodeTypes = {
  trigger: TriggerNode,
  condition: ConditionNode,
  action: ActionNode,
}

export function Flows({ token }) {
  const [automations, setAutomations] = useState([])
  const [objects, setObjects] = useState([])
  const [editor, setEditor] = useState(null)
  const [selectedId, setSelectedId] = useState("trigger")
  const [executions, setExecutions] = useState([])
  const [error, setError] = useState("")

  async function reload() {
    const [listed, defined] = await Promise.all([
      api(token, "GET", "/api/automations"),
      api(token, "GET", "/api/meta/objects"),
    ])
    setAutomations(listed.automations)
    setObjects(defined.objects)
  }

  useEffect(() => {
    let gone = false
    Promise.all([
      api(token, "GET", "/api/automations"),
      api(token, "GET", "/api/meta/objects"),
    ]).then(([listed, defined]) => {
      if (gone) return
      setAutomations(listed.automations)
      setObjects(defined.objects)
    }).catch((err) => {
      if (!gone) setError(err.message)
    })
    return () => { gone = true }
  }, [token])

  function openDraft() {
    const object = objects[0]?.name || ""
    const graph = workflowToGraph({ label: "", event: "record.created", object, conditions: [], actions: [] })
    setEditor({
      name: "",
      saved: false,
      version: 0,
      mode: "event",
      event: "record.created",
      object,
      every: "15m",
      nodes: graph.nodes,
      edges: graph.edges,
    })
    setSelectedId("trigger")
    setExecutions([])
    setError("")
  }

  async function openSaved(name) {
    setError("")
    try {
      const workflow = await api(token, "GET", `/api/automations/${name}`)
      const graph = workflowToGraph(workflow)
      setEditor({
        name: workflow.name,
        saved: true,
        version: workflow.version,
        mode: workflow.every ? "schedule" : "event",
        event: workflow.event || "record.created",
        object: workflow.object || "",
        every: workflow.every || "15m",
        nodes: graph.nodes,
        edges: graph.edges,
      })
      setSelectedId("trigger")
      const history = await api(token, "GET", `/api/automations/${name}/executions`)
      setExecutions(history.executions)
    } catch (err) {
      setError(err.message)
    }
  }

  function patchEditor(patch) {
    setEditor((current) => {
      const next = { ...current, ...patch }
      const mode = next.mode
      return {
        ...next,
        nodes: next.nodes.map((node) => node.id === "trigger" ? {
          ...node,
          data: {
            ...node.data,
            event: mode === "schedule" ? "" : next.event,
            object: mode === "schedule" ? "" : next.object,
            every: mode === "schedule" ? next.every : "",
          },
        } : node),
      }
    })
  }

  function patchNode(id, data) {
    setEditor((current) => ({
      ...current,
      nodes: current.nodes.map((node) => (node.id === id ? { ...node, data: { ...node.data, ...data } } : node)),
    }))
  }

  const onNodesChange = useCallback((changes) => {
    setEditor((current) => current ? { ...current, nodes: applyNodeChanges(changes, current.nodes) } : current)
  }, [])
  const onEdgesChange = useCallback((changes) => {
    setEditor((current) => current ? { ...current, edges: applyEdgeChanges(changes, current.edges) } : current)
  }, [])
  const onConnect = useCallback((connection) => {
    setEditor((current) => {
      if (!current || !connectionAllowed(current.nodes, current.edges, connection)) return current
      const handle = connection.sourceHandle || ""
      return {
        ...current,
        edges: [...current.edges, {
          id: `${connection.source}-${handle}-${connection.target}`,
          source: connection.source,
          target: connection.target,
          ...(handle ? { sourceHandle: handle } : {}),
        }],
      }
    })
  }, [])
  const isValidConnection = useCallback((connection) => {
    return editor ? connectionAllowed(editor.nodes, editor.edges, connection) : false
  }, [editor])

  function addStep(kind) {
    const id = `${kind}-${crypto.randomUUID()}`
    const data = kind === "condition"
      ? { field: "", op: "eq", value: "" }
      : kind === "approval"
        ? { type: "approval", object: "", url: "", user: "", recordMode: "", recordField: "", rows: [] }
        : { type: "create", object: editor.object, url: "", user: "", recordMode: "", recordField: "", rows: [], ...(kind === "else" ? { path: "else" } : {}) }
    const node = { id, type: kind === "condition" ? "condition" : "action", data }
    const graph = kind === "condition"
      ? insertBefore(editor.nodes, editor.edges, node, "action")
      : kind === "else"
        ? appendElse(editor.nodes, editor.edges, node)
        : appendNode(editor.nodes, editor.edges, node)
    patchEditor(graph)
    setSelectedId(id)
  }

  function deleteStep(id) {
    try {
      const graph = removeNode(editor.nodes, editor.edges, id)
      patchEditor(graph)
      setSelectedId("trigger")
    } catch (err) {
      setError(err.message)
    }
  }

  async function save(event) {
    event.preventDefault()
    setError("")
    const trigger = editor.nodes.find((node) => node.id === "trigger")
    const label = trigger?.data.label?.trim() || ""
    if (!label) {
      setError("label is required")
      return
    }
    if (!editor.saved && !editor.name.trim()) {
      setError("a flow needs a name")
      return
    }
    if (editor.mode === "event" && !editor.object) {
      setError("a flow needs an object")
      return
    }
    if (editor.mode === "schedule" && !editor.every.trim()) {
      setError("a schedule needs an interval")
      return
    }
    try {
      const source = objects.find((object) => object.name === editor.object)
      const parsed = graphToWorkflow(editor.nodes, editor.edges, {
        every: editor.mode === "schedule" ? editor.every.trim() : "",
        object: editor.object,
        fields: source?.fields || [],
        fieldsByObject: Object.fromEntries(objects.map((object) => [object.name, object.fields])),
      })
      const saved = editor.saved
        ? await api(token, "PATCH", `/api/automations/${editor.name}`, {
          version: editor.version,
          label,
          conditions: parsed.conditions,
          actions: parsed.actions,
          else: parsed.else,
        })
        : await api(token, "POST", "/api/automations", {
          name: editor.name.trim(),
          label,
          event: editor.mode === "event" ? editor.event : "",
          object: editor.mode === "event" ? editor.object : "",
          every: editor.mode === "schedule" ? editor.every.trim() : "",
          conditions: parsed.conditions,
          actions: parsed.actions,
          else: parsed.else,
        })
      const graph = workflowToGraph(saved)
      setSelectedId("trigger")
      setEditor({
        name: saved.name,
        saved: true,
        version: saved.version,
        mode: saved.every ? "schedule" : "event",
        event: saved.event || editor.event,
        object: saved.object || "",
        every: saved.every || editor.every,
        nodes: graph.nodes,
        edges: graph.edges,
      })
      await reload()
      const history = await api(token, "GET", `/api/automations/${saved.name}/executions`)
      setExecutions(history.executions)
    } catch (err) {
      setError(err.message)
    }
  }

  async function removeFlow() {
    setError("")
    try {
      await api(token, "DELETE", `/api/automations/${editor.name}`)
      setEditor(null)
      setExecutions([])
      await reload()
    } catch (err) {
      setError(err.message)
    }
  }

  if (!editor) {
    return (
      <main>
        <div className="title-row">
          <h1>Flows</h1>
          <Button onClick={openDraft}>New flow</Button>
        </div>
        <Notice tone="error">{error}</Notice>
        {automations.length === 0 && <Notice>No flows yet.</Notice>}
        <ul className="object-list">
          {automations.map((flow) => (
            <li key={flow.id}>
              <Button onClick={() => openSaved(flow.name)}>
                <strong>{flow.label}</strong>
                <span>{flow.every ? `every ${flow.every}` : `${flow.event} · ${flow.object}`}</span>
              </Button>
            </li>
          ))}
        </ul>
      </main>
    )
  }

  const selected = editor.nodes.find((node) => node.id === selectedId) || editor.nodes[0]
  const schedule = editor.mode === "schedule"

  return (
    <main>
      <div className="title-row">
        <Button variant="text" onClick={() => { setEditor(null); setError("") }}>Flows</Button>
        <h1>{selected?.data.label || editor.name || "New flow"}</h1>
        {editor.saved && <span className="version">version {editor.version}</span>}
      </div>
      <Notice tone="error">{error}</Notice>
      <div className="flow-layout">
        <div className="flow-canvas">
          <ReactFlow
            nodes={editor.nodes.map((node) => ({ ...node, selected: node.id === selectedId }))}
            edges={editor.edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            isValidConnection={isValidConnection}
            nodeTypes={nodeTypes}
            onNodeClick={(_, node) => setSelectedId(node.id)}
            onInit={(instance) => instance.fitView()}
            colorMode="light"
            deleteKeyCode={null}
            proOptions={{ hideAttribution: false }}
          >
            <Background />
            <Controls />
            <MiniMap />
          </ReactFlow>
        </div>
        <form className="panel" onSubmit={save}>
          <Notice>Conditions run first. A match follows the actions. One approval on that path waits for a person. A miss follows the other path.</Notice>
          {selected?.type === "trigger" && (
            <>
              <h2>Trigger</h2>
              {!editor.saved && (
                <label>
                  Name
                  <input value={editor.name} onChange={(event) => patchEditor({ name: event.target.value })} required />
                </label>
              )}
              <label>
                Label
                <input value={selected.data.label} onChange={(event) => patchNode("trigger", { label: event.target.value })} required />
              </label>
              {!editor.saved && (
                <label>
                  Runs
                  <select value={editor.mode} onChange={(event) => patchEditor({ mode: event.target.value })}>
                    <option value="event">When a record changes</option>
                    <option value="schedule">On a schedule</option>
                  </select>
                </label>
              )}
              {editor.mode === "event" ? (
                <>
                  <label>
                    Event
                    <select value={editor.event} disabled={editor.saved} onChange={(event) => patchEditor({ event: event.target.value })}>
                      {events.map((name) => <option key={name}>{name}</option>)}
                    </select>
                  </label>
                  <label>
                    Object
                    <select value={editor.object} disabled={editor.saved} onChange={(event) => patchEditor({ object: event.target.value })}>
                      {objects.map((object) => <option key={object.name} value={object.name}>{object.label}</option>)}
                    </select>
                  </label>
                </>
              ) : (
                <label>
                  Every
                  <input value={editor.every} disabled={editor.saved} onChange={(event) => patchEditor({ every: event.target.value })} placeholder="15m" />
                </label>
              )}
            </>
          )}
          {selected?.type === "condition" && (
            <ConditionForm node={selected} fields={objects.find((object) => object.name === editor.object)?.fields || []} onChange={(data) => patchNode(selected.id, data)} onDelete={() => deleteStep(selected.id)} />
          )}
          {selected?.type === "action" && (
            <ActionForm node={selected} objects={objects} source={editor.object} onChange={(data) => patchNode(selected.id, data)} onDelete={() => deleteStep(selected.id)} />
          )}
          <div className="actions">
            {!schedule && <Button type="button" variant="secondary" onClick={() => addStep("condition")}>Add condition</Button>}
            <Button type="button" variant="secondary" onClick={() => addStep("action")}>Add action</Button>
            <Button type="button" variant="secondary" onClick={() => addStep("approval")}>Add approval</Button>
            {!schedule && <Button type="button" variant="secondary" onClick={() => addStep("else")}>Add else</Button>}
            <Button>Save</Button>
          </div>
          {editor.saved && <Button type="button" variant="secondary" onClick={removeFlow}>Delete flow</Button>}
          {editor.saved && (
            <>
              <h2>Executions</h2>
              {executions.length === 0 && <Notice>None yet.</Notice>}
              <ul className="executions">
                {executions.map((item) => (
                  <li key={item.id}><span>{item.status}{item.path ? ` · ${item.path}` : ""}</span><span className="empty">{item.created_at}</span></li>
                ))}
              </ul>
            </>
          )}
        </form>
      </div>
    </main>
  )
}

function ConditionForm({ node, fields, onChange, onDelete }) {
  return (
    <>
      <h2>Condition</h2>
      <label>
        Field
        <input list="flow-fields" value={node.data.field} onChange={(event) => onChange({ field: event.target.value })} />
      </label>
      <datalist id="flow-fields">
        {fields.map((field) => <option key={field.name} value={field.name} />)}
      </datalist>
      <label>
        Operator
        <select value={node.data.op} onChange={(event) => onChange({ op: event.target.value })}>
          {operators.map((op) => <option key={op}>{op}</option>)}
        </select>
      </label>
      <label>
        Value
        <input value={node.data.value} onChange={(event) => onChange({ value: event.target.value })} />
      </label>
      <Button type="button" variant="secondary" onClick={onDelete}>Delete step</Button>
    </>
  )
}

function ActionForm({ node, objects, source, onChange, onDelete }) {
  const data = node.data
  function patch(next) {
    onChange({ ...data, ...next })
  }
  function patchRow(index, next) {
    patch({ rows: data.rows.map((row, i) => (i === index ? { ...row, ...next } : row)) })
  }
  return (
    <>
      <h2>{data.path === "else" ? "Else" : data.type === "approval" ? "Approval" : "Action"}</h2>
      <label>
        Type
        <select value={data.type} onChange={(event) => patch({ type: event.target.value })}>
          {actionTypes.map((type) => <option key={type}>{type}</option>)}
        </select>
      </label>
      {data.type === "approval" ? (
        <label>
          User
          <input type="email" value={data.user || ""} onChange={(event) => patch({ user: event.target.value })} placeholder="member@example.com" />
        </label>
      ) : data.type === "http" ? (
        <label>
          URL
          <input value={data.url} onChange={(event) => patch({ url: event.target.value })} />
        </label>
      ) : (
        <label>
          Object
          <select value={data.object} onChange={(event) => patch({ object: event.target.value })}>
            <option value="">{source ? `This ${source}` : "Choose"}</option>
            {objects.map((object) => <option key={object.name} value={object.name}>{object.label}</option>)}
          </select>
        </label>
      )}
      {data.type !== "http" && data.type !== "create" && data.type !== "approval" && (
        <label>
          Record
          <select
            value={data.recordMode === "field" ? "field" : ""}
            onChange={(event) => patch({ recordMode: event.target.value })}
          >
            <option value="">The record that triggered this flow</option>
            <option value="field">The id stored in a field</option>
          </select>
        </label>
      )}
      {data.recordMode === "field" && data.type !== "approval" && (
        <label>
          Field on the triggering record
          <input value={data.recordField} onChange={(event) => patch({ recordField: event.target.value })} />
        </label>
      )}
      {data.type !== "delete" && data.type !== "approval" && data.rows.map((row, index) => (
        <div className="field-row" key={index}>
          <input aria-label="Value field" value={row.field} onChange={(event) => patchRow(index, { field: event.target.value })} placeholder="field" />
          <select aria-label="Value mode" value={row.mode} onChange={(event) => patchRow(index, { mode: event.target.value })}>
            <option value="literal">Literal</option>
            <option value="field">Copy field</option>
            <option value="record">Record id</option>
          </select>
          {row.mode !== "record" && (
            <input aria-label="Value" value={row.value} onChange={(event) => patchRow(index, { value: event.target.value })} />
          )}
        </div>
      ))}
      {data.type !== "delete" && data.type !== "approval" && (
        <Button type="button" variant="secondary" onClick={() => patch({ rows: [...data.rows, { field: "", mode: "literal", value: "" }] })}>Add value</Button>
      )}
      <Button type="button" variant="secondary" onClick={onDelete}>Delete step</Button>
    </>
  )
}

function TriggerNode({ data }) {
  const when = data.every ? `Every ${data.every}` : `${data.event || "record event"}${data.object ? ` · ${data.object}` : ""}`
  return (
    <div className="flow-node trigger">
      <div className="flow-kind">Trigger</div>
      <strong>{data.label || "Untitled"}</strong>
      <div>{when}</div>
      <Handle type="source" position={Position.Bottom} />
      <Handle id="else" type="source" position={Position.Right} />
    </div>
  )
}

function ConditionNode({ data }) {
  return (
    <div className="flow-node condition">
      <Handle type="target" position={Position.Top} />
      <div className="flow-kind">Condition</div>
      <strong>{data.field || "Field"} {data.op} {data.value}</strong>
      <Handle type="source" position={Position.Bottom} />
      <Handle id="else" type="source" position={Position.Right} />
    </div>
  )
}

function ActionNode({ data }) {
  const detail = data.type === "http" ? data.url || "URL" : data.type === "approval" ? data.user || "User" : data.object || "This record"
  const other = data.path === "else"
  const approval = !other && data.type === "approval"
  const className = other ? "flow-node else" : approval ? "flow-node approval" : "flow-node action"
  return (
    <div className={className}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-kind">{other ? "Else" : approval ? "Approval" : "Action"}</div>
      <strong>{data.type}</strong>
      <div>{detail}</div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}
