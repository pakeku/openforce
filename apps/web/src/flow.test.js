import assert from "node:assert/strict"
import test from "node:test"
import { appendNode, connectionAllowed, graphToWorkflow, insertBefore, removeNode, workflowToGraph } from "./flow.js"

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

function actionData(type) {
  return { type, object: "", url: "https://example.com/hook", recordMode: "", recordField: "", rows: [] }
}
