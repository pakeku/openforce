import assert from "node:assert/strict"
import test from "node:test"
import { appendElse, appendNode, connectionAllowed, graphToWorkflow, insertBefore, removeNode, workflowToGraph } from "./flow.js"

const fields = [{ name: "employees", type: "number" }, { name: "name", type: "text" }]
const fieldsByObject = {
  company: fields,
  task: [{ name: "name", type: "text" }, { name: "company", type: "relation" }],
}

test("a record workflow round-trips through the canvas", () => {
  const workflow = {
    label: "Follow up",
    event: "record.created",
    object: "company",
    conditions: [{ field: "employees", op: "gte", value: 3 }],
    actions: [
      { type: "create", object: "task", data: { name: "Follow up", company: { record: true } } },
      { type: "http", url: "https://example.com/hook", data: { name: { field: "name" } } },
    ],
  }
  const graph = workflowToGraph(workflow)
  const back = graphToWorkflow(graph.nodes, graph.edges, { object: "company", fields, fieldsByObject })
  assert.deepEqual(back.conditions, workflow.conditions)
  assert.deepEqual(back.actions, workflow.actions)
  assert.deepEqual(back.else, [])
})

test("a schedule workflow round-trips without conditions", () => {
  const workflow = {
    label: "Ping",
    every: "15m",
    conditions: [],
    actions: [{ type: "http", url: "https://example.com/hook" }],
  }
  const graph = workflowToGraph(workflow)
  const back = graphToWorkflow(graph.nodes, graph.edges, { every: "15m", fields: [], fieldsByObject: {} })
  assert.deepEqual(back.conditions, [])
  assert.deepEqual(back.actions, [{ type: "http", url: "https://example.com/hook" }])
  assert.deepEqual(back.else, [])
})

test("a schedule rejects a condition and an update", () => {
  const graph = workflowToGraph({
    label: "Ping",
    every: "15m",
    conditions: [{ field: "name", op: "eq", value: "Acme" }],
    actions: [{ type: "http", url: "https://example.com/hook" }],
  })
  assert.throws(
    () => graphToWorkflow(graph.nodes, graph.edges, { every: "15m" }),
    /a schedule has no conditions/,
  )
  const update = workflowToGraph({
    label: "Ping",
    every: "15m",
    actions: [{ type: "update", data: { name: "Acme" } }],
  })
  assert.throws(
    () => graphToWorkflow(update.nodes, update.edges, { every: "15m", object: "company", fields, fieldsByObject }),
    /a schedule can create a record or POST JSON/,
  )
})

test("the chain rejects a loop, a gap, and a missing action", () => {
  const loop = workflowToGraph({ label: "Loop", actions: [{ type: "delete" }] })
  loop.edges.push({ id: "back", source: "action-0", target: "trigger" })
  assert.throws(() => graphToWorkflow(loop.nodes, loop.edges), /a flow cannot loop/)
  const gap = workflowToGraph({ label: "Gap", actions: [{ type: "delete" }] })
  assert.throws(() => graphToWorkflow(gap.nodes, []), /every step connects to the trigger/)
  const empty = workflowToGraph({ label: "Empty", actions: [] })
  assert.throws(() => graphToWorkflow(empty.nodes, empty.edges), /actions are required/)
})

test("a condition is inserted before the first action", () => {
  let graph = workflowToGraph({
    label: "Chain",
    actions: [{ type: "create", object: "task", data: { name: "Follow up" } }],
  })
  graph = insertBefore(graph.nodes, graph.edges, {
    id: "condition-new",
    type: "condition",
    data: { field: "employees", op: "gte", value: "3" },
  }, "action")
  const back = graphToWorkflow(graph.nodes, graph.edges, { object: "company", fields, fieldsByObject })
  assert.deepEqual(back.conditions, [{ field: "employees", op: "gte", value: 3 }])
  assert.equal(back.actions[0].type, "create")
})

test("adding and removing a step keeps one chain", () => {
  let graph = workflowToGraph({ label: "Chain", actions: [{ type: "delete" }] })
  graph = appendNode(graph.nodes, graph.edges, {
    id: "action-extra",
    type: "action",
    data: actionData("http"),
  })
  graph = removeNode(graph.nodes, graph.edges, "action-0")
  const back = graphToWorkflow(graph.nodes, graph.edges)
  assert.deepEqual(back.actions, [{ type: "http", url: "https://example.com/hook" }])
})

test("a connection can join the tail to a loose step and cannot loop", () => {
  const graph = workflowToGraph({ label: "Chain", actions: [{ type: "delete" }] })
  const loose = { id: "action-extra", type: "action", data: actionData("http") }
  const withLoose = { nodes: [...graph.nodes, loose], edges: graph.edges }
  assert.equal(connectionAllowed(withLoose.nodes, withLoose.edges, { source: "action-0", target: "action-extra" }), true)
  assert.equal(connectionAllowed(withLoose.nodes, withLoose.edges, { source: "trigger", target: "action-extra" }), false)
  assert.equal(connectionAllowed(withLoose.nodes, withLoose.edges, { source: "action-0", target: "trigger" }), false)
})

test("else round-trips behind the conditions", () => {
  const workflow = {
    label: "Fork",
    event: "record.created",
    object: "company",
    conditions: [{ field: "employees", op: "gte", value: 3 }],
    actions: [
      { type: "update", data: { industry: "Finance" } },
      { type: "http", url: "https://example.com/match" },
    ],
    else: [
      { type: "create", object: "task", data: { name: "Miss", company: { record: true } } },
      { type: "delete" },
    ],
  }
  const graph = workflowToGraph(workflow)
  const back = graphToWorkflow(graph.nodes, graph.edges, { object: "company", fields, fieldsByObject })
  assert.deepEqual(back.conditions, workflow.conditions)
  assert.deepEqual(back.actions, workflow.actions)
  assert.deepEqual(back.else, workflow.else)
  const elseEdge = graph.edges.find((edge) => edge.sourceHandle === "else")
  assert.equal(elseEdge.source, "condition-0")
  assert.ok(graph.nodes.find((node) => node.id === "action-0").position.x < graph.nodes.find((node) => node.id === "else-0").position.x)
})

test("a condition inserted later still lands before both paths", () => {
  let graph = workflowToGraph({
    label: "Fork",
    conditions: [{ field: "employees", op: "gte", value: 3 }],
    actions: [{ type: "delete" }],
  })
  graph = appendElse(graph.nodes, graph.edges, {
    id: "else-extra",
    type: "action",
    data: { ...actionData("http"), path: "else" },
  })
  graph = insertBefore(graph.nodes, graph.edges, {
    id: "condition-new",
    type: "condition",
    data: { field: "name", op: "eq", value: "Acme" },
  }, "action")
  const back = graphToWorkflow(graph.nodes, graph.edges, { object: "company", fields, fieldsByObject })
  assert.deepEqual(back.conditions, [
    { field: "employees", op: "gte", value: 3 },
    { field: "name", op: "eq", value: "Acme" },
  ])
  assert.deepEqual(back.actions, [{ type: "delete" }])
  assert.deepEqual(back.else, [{ type: "http", url: "https://example.com/hook" }])
  assert.equal(graph.edges.find((edge) => edge.sourceHandle === "else").source, "condition-new")
})

test("removing the condition keeps the other path in front of the trigger", () => {
  let graph = workflowToGraph({
    label: "Fork",
    conditions: [{ field: "name", op: "eq", value: "Acme" }],
    actions: [{ type: "delete" }],
    else: [{ type: "http", url: "https://example.com/hook" }],
  })
  graph = removeNode(graph.nodes, graph.edges, "condition-0")
  const back = graphToWorkflow(graph.nodes, graph.edges)
  assert.deepEqual(back.conditions, [])
  assert.deepEqual(back.actions, [{ type: "delete" }])
  assert.deepEqual(back.else, [{ type: "http", url: "https://example.com/hook" }])
  assert.equal(graph.edges.find((edge) => edge.sourceHandle === "else").source, "trigger")
})

test("one approval round-trips on the match path", () => {
  const workflow = {
    label: "Review",
    event: "record.created",
    object: "company",
    conditions: [{ field: "employees", op: "gte", value: 3 }],
    actions: [
      { type: "create", object: "task", data: { name: "Earlier" } },
      { type: "approval", user: "mia@example.com" },
      { type: "http", url: "https://example.com/later" },
    ],
    else: [{ type: "create", object: "task", data: { name: "Miss" } }],
  }
  const graph = workflowToGraph(workflow)
  const back = graphToWorkflow(graph.nodes, graph.edges, { object: "company", fields, fieldsByObject })
  assert.equal("position" in back, false)
  assert.equal(back.actions[1].position, undefined)
  assert.deepEqual(back.conditions, workflow.conditions)
  assert.deepEqual(back.actions, workflow.actions)
  assert.deepEqual(back.else, workflow.else)
  assert.equal(graph.edges.find((edge) => edge.sourceHandle === "else").source, "condition-0")
  const clock = workflowToGraph({
    label: "Clock",
    every: "1d",
    actions: [{ type: "approval", user: "mia@example.com" }],
  })
  const scheduled = graphToWorkflow(clock.nodes, clock.edges, { every: "1d", fields: [], fieldsByObject: {} })
  assert.deepEqual(scheduled.actions, [{ type: "approval", user: "mia@example.com" }])
  assert.deepEqual(scheduled.else, [])
})

test("a second approval or an else approval is rejected", () => {
  const second = workflowToGraph({
    label: "Twice",
    actions: [
      { type: "approval", user: "mia@example.com" },
      { type: "approval", user: "ada@example.com" },
    ],
  })
  assert.throws(
    () => graphToWorkflow(second.nodes, second.edges),
    /a flow has one approval/,
  )
  const other = workflowToGraph({
    label: "Other",
    actions: [{ type: "delete" }],
    else: [{ type: "approval", user: "mia@example.com" }],
  })
  assert.throws(
    () => graphToWorkflow(other.nodes, other.edges),
    /an approval stays on the match path/,
  )
})

test("a schedule rejects else", () => {
  const graph = workflowToGraph({
    label: "Ping",
    every: "15m",
    actions: [{ type: "http", url: "https://example.com/hook" }],
    else: [{ type: "create", object: "task", data: { name: "Nope" } }],
  })
  assert.throws(
    () => graphToWorkflow(graph.nodes, graph.edges, { every: "15m", fields: [], fieldsByObject: {} }),
    /a schedule has no else/,
  )
})

test("a loop action round-trips through the canvas", () => {
  const taskFields = [
    { name: "name", type: "text" },
    { name: "status", type: "text" },
    { name: "company", type: "relation" },
  ]
  const lookup = {
    company: fields,
    task: taskFields,
    note: [{ name: "name", type: "text" }],
  }
  const workflow = {
    label: "Close tasks",
    event: "record.created",
    object: "company",
    conditions: [{ field: "name", op: "eq", value: "Acme" }],
    actions: [{
      type: "loop",
      object: "task",
      limit: 2,
      where: [
        { field: "status", op: "eq", value: "open" },
        { field: "company", op: "eq", value: { record: true } },
      ],
      actions: [
        { type: "update", data: { status: "done" } },
        { type: "create", object: "note", data: { name: { field: "name" } } },
      ],
    }],
    else: [{
      type: "loop",
      object: "task",
      limit: 10,
      actions: [{ type: "delete" }],
    }],
  }
  const graph = workflowToGraph(workflow)
  const back = graphToWorkflow(graph.nodes, graph.edges, { object: "company", fields, fieldsByObject: lookup })
  assert.deepEqual(back.conditions, workflow.conditions)
  assert.deepEqual(back.actions, workflow.actions)
  assert.deepEqual(back.else, workflow.else)
  const open = workflowToGraph({
    label: "All",
    actions: [{ type: "loop", object: "task", actions: [{ type: "delete" }] }],
  })
  const saved = graphToWorkflow(open.nodes, open.edges, { object: "company", fields, fieldsByObject: lookup })
  assert.equal(saved.actions[0].limit, 25)
  assert.equal(saved.actions[0].where, undefined)
})

test("a loop stays on a record workflow and inside its own limit", () => {
  const lookup = { task: [{ name: "name", type: "text" }] }
  const schedule = workflowToGraph({
    label: "Nightly",
    every: "1d",
    actions: [{ type: "loop", object: "task", limit: 25, actions: [{ type: "delete" }] }],
  })
  assert.throws(
    () => graphToWorkflow(schedule.nodes, schedule.edges, { every: "1d", fields: [], fieldsByObject: lookup }),
    /a schedule can create a record or POST JSON/,
  )
  const graph = workflowToGraph({
    label: "Close",
    actions: [{ type: "loop", object: "task", limit: 2, actions: [{ type: "delete" }] }],
  })
  const node = graph.nodes.find((item) => item.id === "action-0")
  node.data.steps = [{ type: "loop", object: "task", steps: [{ type: "delete" }] }]
  assert.throws(
    () => graphToWorkflow(graph.nodes, graph.edges, { fieldsByObject: lookup }),
    /invalid action/,
  )
  node.data.steps = [{ type: "approval", user: "mia@example.com" }]
  assert.throws(
    () => graphToWorkflow(graph.nodes, graph.edges, { fieldsByObject: lookup }),
    /invalid action/,
  )
  node.data.steps = []
  assert.throws(
    () => graphToWorkflow(graph.nodes, graph.edges, { fieldsByObject: lookup }),
    /actions are required/,
  )
  node.data.steps = [{ type: "delete" }]
  node.data.object = ""
  assert.throws(
    () => graphToWorkflow(graph.nodes, graph.edges, { fieldsByObject: lookup }),
    /a loop needs an object/,
  )
  node.data.object = "task"
  for (const limit of ["0", "51", "1.5"]) {
    node.data.limit = limit
    assert.throws(
      () => graphToWorkflow(graph.nodes, graph.edges, { fieldsByObject: lookup }),
      /invalid limit/,
    )
  }
})

function actionData(type) {
  return { type, object: "", url: "https://example.com/hook", recordMode: "", recordField: "", rows: [] }
}
