export function workflowToGraph(workflow) {
  const nodes = [
    {
      id: "trigger",
      type: "trigger",
      position: { x: 80, y: 0 },
      data: {
        label: workflow.label || "",
        event: workflow.event || "",
        object: workflow.object || "",
        every: workflow.every || "",
      },
      deletable: false,
    },
  ]
  const edges = []
  let previous = "trigger"
  let y = 160
  for (const [index, condition] of (workflow.conditions || []).entries()) {
    const id = `condition-${index}`
    nodes.push({
      id,
      type: "condition",
      position: { x: 80, y },
      data: {
        field: condition.field || "",
        op: condition.op || "eq",
        value: condition.value == null ? "" : String(condition.value),
      },
    })
    edges.push({ id: `${previous}-${id}`, source: previous, target: id })
    previous = id
    y += 160
  }
  const fork = previous
  const branchY = y
  for (const [index, action] of (workflow.actions || []).entries()) {
    const id = `action-${index}`
    nodes.push({
      id,
      type: "action",
      position: { x: 80, y },
      data: actionToNodeData(action),
    })
    edges.push({ id: `${previous}-${id}`, source: previous, target: id })
    previous = id
    y += 160
  }
  let elseY = branchY
  let elsePrevious = ""
  for (const [index, action] of (workflow.else || []).entries()) {
    const id = `else-${index}`
    nodes.push({
      id,
      type: "action",
      position: { x: 420, y: elseY },
      data: { ...actionToNodeData(action), path: "else" },
    })
    if (index === 0) {
      edges.push({ id: `${fork}-else-${id}`, source: fork, target: id, sourceHandle: "else", label: "else" })
    } else {
      edges.push({ id: `${elsePrevious}-${id}`, source: elsePrevious, target: id })
    }
    elsePrevious = id
    elseY += 160
  }
  return { nodes, edges }
}

export function graphToWorkflow(nodes, edges, options = {}) {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  if (!byId.has("trigger")) throw new Error("a flow starts with a trigger")
  const primary = new Map()
  const elses = new Map()
  const incoming = new Map()
  for (const edge of edges) {
    if (edge.sourceHandle === "else") {
      if (elses.has(edge.source)) throw new Error("each step has one next step")
      elses.set(edge.source, edge.target)
    } else {
      if (primary.has(edge.source)) throw new Error("each step has one next step")
      primary.set(edge.source, edge.target)
    }
    if (incoming.has(edge.target)) throw new Error("each step has one previous step")
    incoming.set(edge.target, edge.source)
  }
  const seen = new Set()
  const ordered = []
  let current = "trigger"
  while (primary.has(current)) {
    const next = primary.get(current)
    if (next === "trigger" || seen.has(next)) throw new Error("a flow cannot loop")
    const node = byId.get(next)
    if (!node) throw new Error("every step connects to the trigger")
    seen.add(next)
    ordered.push(node)
    current = next
  }
  const fields = options.fields || []
  const fieldsByObject = options.fieldsByObject || {}
  const sourceObject = options.object || ""
  const every = options.every || ""
  const conditions = []
  const actions = []
  let seenAction = false
  let fork = "trigger"
  for (const node of ordered) {
    if (node.type === "condition") {
      if (seenAction) throw new Error("conditions come before actions")
      if (!node.data.field) throw new Error("a condition needs a field")
      if (!node.data.op) throw new Error("a condition needs an operator")
      const type = fields.find((field) => field.name === node.data.field)?.type
      conditions.push({
        field: node.data.field,
        op: node.data.op,
        value: literal(node.data.value, type),
      })
      fork = node.id
      continue
    }
    if (node.type !== "action") throw new Error("unknown step")
    seenAction = true
    actions.push(actionFromNode(node.data, sourceObject, fieldsByObject))
  }
  for (const source of elses.keys()) {
    if (source !== fork) throw new Error("the other path starts after the conditions")
  }
  const elseActions = []
  if (elses.has(fork)) {
    let next = elses.get(fork)
    const local = new Set()
    while (next) {
      if (next === "trigger" || local.has(next) || seen.has(next)) throw new Error("a flow cannot loop")
      const node = byId.get(next)
      if (!node) throw new Error("every step connects to the trigger")
      if (node.type === "condition") throw new Error("conditions come before actions")
      if (node.type !== "action") throw new Error("unknown step")
      local.add(next)
      seen.add(next)
      elseActions.push(actionFromNode(node.data, sourceObject, fieldsByObject))
      next = primary.get(next)
    }
  }
  for (const node of nodes) {
    if (node.id !== "trigger" && !seen.has(node.id)) throw new Error("every step connects to the trigger")
  }
  if (actions.length === 0) throw new Error("actions are required")
  if (every && conditions.length > 0) throw new Error("a schedule has no conditions")
  if (every && elseActions.length > 0) throw new Error("a schedule has no else")
  const blocked = (action) => action.type === "update" || action.type === "delete"
  if (every && (actions.some(blocked) || elseActions.some(blocked))) {
    throw new Error("a schedule can create a record or POST JSON")
  }
  if (actions.filter((action) => action.type === "approval").length > 1) {
    throw new Error("a flow has one approval")
  }
  if (elseActions.some((action) => action.type === "approval")) {
    throw new Error("an approval stays on the match path")
  }
  return { conditions, actions, else: elseActions }
}

export function insertBefore(nodes, edges, node, type) {
  let current = "trigger"
  const seen = new Set()
  while (!seen.has(current)) {
    seen.add(current)
    const next = primaryTarget(edges, current)
    if (!next) break
    const target = nodes.find((item) => item.id === next)
    if (target?.type === type) {
      const previous = nodes.find((item) => item.id === current)
      const placed = {
        ...node,
        position: node.position || { x: 80, y: (previous?.position.y || 0) + 160 },
      }
      let nextEdges = edges.filter((edge) => !(edge.source === current && edge.target === next && edge.sourceHandle !== "else"))
      nextEdges = nextEdges.map((edge) => (
        edge.source === current && edge.sourceHandle === "else"
          ? { ...edge, id: `${placed.id}-else-${edge.target}`, source: placed.id }
          : edge
      ))
      nextEdges.push({ id: `${current}-${placed.id}`, source: current, target: placed.id })
      nextEdges.push({ id: `${placed.id}-${next}`, source: placed.id, target: next })
      return { nodes: [...nodes, placed], edges: nextEdges }
    }
    current = next
  }
  return appendNode(nodes, edges, node)
}

export function appendNode(nodes, edges, node) {
  const tail = chainTail(edges)
  const previous = nodes.find((item) => item.id === tail)
  const placed = {
    ...node,
    position: node.position || { x: 80, y: (previous?.position.y || 0) + 160 },
  }
  let nextEdges = edges
  if (node.type === "condition") {
    nextEdges = edges.map((edge) => (
      edge.source === tail && edge.sourceHandle === "else"
        ? { ...edge, id: `${placed.id}-else-${edge.target}`, source: placed.id }
        : edge
    ))
  }
  return {
    nodes: [...nodes, placed],
    edges: [...nextEdges, { id: `${tail}-${placed.id}`, source: tail, target: placed.id }],
  }
}

export function appendElse(nodes, edges, node) {
  const fork = forkOf(nodes, edges)
  const tail = elseTail(edges, fork)
  const previous = nodes.find((item) => item.id === tail)
  const firstAction = nodes.find((item) => item.id === primaryTarget(edges, fork))
  const y = tail === fork
    ? (firstAction?.position.y ?? (previous?.position.y || 0) + 160)
    : (previous?.position.y || 0) + 160
  const x = tail === fork ? 420 : (previous?.position.x ?? 420)
  const placed = {
    ...node,
    position: node.position || { x, y },
  }
  const edge = tail === fork
    ? { id: `${fork}-else-${placed.id}`, source: fork, target: placed.id, sourceHandle: "else", label: "else" }
    : { id: `${tail}-${placed.id}`, source: tail, target: placed.id }
  return { nodes: [...nodes, placed], edges: [...edges, edge] }
}

export function removeNode(nodes, edges, id) {
  if (id === "trigger") throw new Error("the trigger stays")
  const incoming = edges.find((edge) => edge.target === id)
  const outgoing = edges.filter((edge) => edge.source === id)
  const next = edges.filter((edge) => edge.source !== id && edge.target !== id)
  if (incoming) {
    for (const edge of outgoing) {
      const sourceHandle = edge.sourceHandle || incoming.sourceHandle
      const rebuilt = {
        id: `${incoming.source}-${sourceHandle || "next"}-${edge.target}`,
        source: incoming.source,
        target: edge.target,
      }
      if (sourceHandle) {
        rebuilt.sourceHandle = sourceHandle
        if (sourceHandle === "else") rebuilt.label = "else"
      }
      next.push(rebuilt)
    }
  }
  return { nodes: nodes.filter((node) => node.id !== id), edges: next }
}

export function connectionAllowed(nodes, edges, connection) {
  if (!connection?.source || !connection?.target || connection.source === connection.target) return false
  if (connection.target === "trigger") return false
  const handle = connection.sourceHandle || ""
  if (handle === "else" && forkOf(nodes, edges) !== connection.source) return false
  if (edges.some((edge) => edge.target === connection.target)) return false
  if (edges.some((edge) => edge.source === connection.source && (edge.sourceHandle || "") === handle)) return false
  if (reaches(edges, connection.target, connection.source)) return false
  const source = nodes.find((node) => node.id === connection.source)
  const target = nodes.find((node) => node.id === connection.target)
  if (!source || !target) return false
  if (handle === "else" && target.type !== "action") return false
  if (connection.source !== "trigger" && !reaches(edges, "trigger", connection.source)) return false
  if (reaches(edges, "trigger", connection.target)) return false
  return true
}

function chainTail(edges) {
  let tail = "trigger"
  const seen = new Set()
  while (!seen.has(tail)) {
    seen.add(tail)
    const next = primaryTarget(edges, tail)
    if (!next) return tail
    tail = next
  }
  return tail
}

function forkOf(nodes, edges) {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  let current = "trigger"
  const seen = new Set()
  while (!seen.has(current)) {
    seen.add(current)
    const next = primaryTarget(edges, current)
    if (!next) return current
    const node = byId.get(next)
    if (!node || node.type === "action") return current
    current = next
  }
  return current
}

function elseTail(edges, fork) {
  const first = edges.find((edge) => edge.source === fork && edge.sourceHandle === "else")
  if (!first) return fork
  let current = first.target
  const seen = new Set([fork])
  while (!seen.has(current)) {
    seen.add(current)
    const next = primaryTarget(edges, current)
    if (!next) return current
    current = next
  }
  return current
}

function primaryTarget(edges, source) {
  const edge = edges.find((item) => item.source === source && item.sourceHandle !== "else")
  return edge?.target || ""
}

function reaches(edges, from, to) {
  const outgoing = new Map()
  for (const edge of edges) {
    const next = outgoing.get(edge.source) || []
    next.push(edge.target)
    outgoing.set(edge.source, next)
  }
  const stack = [...(outgoing.get(from) || [])]
  const seen = new Set()
  while (stack.length) {
    const current = stack.pop()
    if (current === to) return true
    if (seen.has(current)) continue
    seen.add(current)
    for (const next of outgoing.get(current) || []) stack.push(next)
  }
  return false
}

function actionToNodeData(action) {
  const rows = []
  for (const [field, value] of Object.entries(action.data || {})) {
    if (value && typeof value === "object" && Object.keys(value).length === 1 && typeof value.field === "string") {
      rows.push({ field, mode: "field", value: value.field })
    } else if (value && typeof value === "object" && value.record === true) {
      rows.push({ field, mode: "record", value: "" })
    } else {
      rows.push({ field, mode: "literal", value: value == null ? "" : String(value) })
    }
  }
  let recordMode = ""
  let recordField = ""
  if (action.record && typeof action.record === "object") {
    if (action.record.record === true) recordMode = "record"
    else if (typeof action.record.field === "string") {
      recordMode = "field"
      recordField = action.record.field
    }
  }
  return {
    type: action.type,
    object: action.object || "",
    url: action.url || "",
    user: action.user || "",
    recordMode,
    recordField,
    rows,
  }
}

function actionFromNode(data, sourceObject, fieldsByObject) {
  const target = data.object || sourceObject
  const lookup = (field) => (fieldsByObject[target] || []).find((item) => item.name === field)?.type
  if (data.type === "http") {
    if (!data.url?.trim()) throw new Error("an http action needs a url")
    const action = { type: "http", url: data.url.trim() }
    if (data.rows?.length) action.data = objectFromRows(data.rows, lookup)
    return action
  }
  if (data.type === "approval") {
    const user = typeof data.user === "string" ? data.user.trim() : ""
    if (!user) throw new Error("an approval needs a user")
    return { type: "approval", user }
  }
  if (data.type !== "create" && data.type !== "update" && data.type !== "delete") {
    throw new Error("invalid action")
  }
  const action = { type: data.type }
  if (data.object?.trim()) action.object = data.object.trim()
  if (data.type === "create" && !action.object) throw new Error("a create action needs an object")
  if (data.recordMode === "record") action.record = { record: true }
  if (data.recordMode === "field") {
    if (!data.recordField) throw new Error("a record field is required")
    action.record = { field: data.recordField }
  }
  if (data.type === "delete") return action
  if (data.type === "update" && !(data.rows?.length)) throw new Error("data is required")
  if (data.rows?.length) action.data = objectFromRows(data.rows, lookup)
  return action
}

function objectFromRows(rows, lookup) {
  const data = {}
  for (const row of rows) {
    if (!row.field) throw new Error("an action value needs a field")
    if (Object.prototype.hasOwnProperty.call(data, row.field)) throw new Error("duplicate field")
    if (row.mode === "field") data[row.field] = { field: row.value }
    else if (row.mode === "record") data[row.field] = { record: true }
    else data[row.field] = literal(row.value, lookup(row.field))
  }
  return data
}

function literal(value, type) {
  if (typeof value !== "string") return value
  if (type === "number" || type === "currency") {
    if (value.trim() === "") return value
    const number = Number(value)
    return Number.isNaN(number) ? value : number
  }
  if (type === "boolean") {
    if (value === "true") return true
    if (value === "false") return false
  }
  return value
}
