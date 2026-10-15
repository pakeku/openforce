import assert from "node:assert/strict"
import test from "node:test"
import { boardColumns, boardMove, cardColumn, cardTitle, cardsInColumn, viewNameOk } from "./board.js"

const options = ["SaaS", "Finance"]

test("lanes follow the enum and add None for a value outside it", () => {
  const records = [
    { id: "a", data: { name: "Acme", industry: "SaaS" } },
    { id: "b", data: { name: "Blank" } },
    { id: "c", data: { industry: "Other" } },
  ]
  assert.deepEqual(boardColumns(options, records, "industry"), [
    { key: "SaaS", label: "SaaS" },
    { key: "Finance", label: "Finance" },
    { key: "", label: "None" },
  ])
  assert.deepEqual(cardsInColumn(records, "industry", "SaaS", options).map((record) => record.id), ["a"])
  assert.deepEqual(cardsInColumn(records, "industry", "", options).map((record) => record.id), ["b", "c"])
  assert.equal(cardColumn(records[0], "industry", options), "SaaS")
  assert.equal(cardColumn(records[1], "industry", options), "")
})

test("a board with every card on an option has no None lane", () => {
  const records = [{ id: "a", data: { industry: "Finance" } }]
  assert.deepEqual(boardColumns(options, records, "industry").map((lane) => lane.key), ["SaaS", "Finance"])
})

test("moving a card writes the enum or clears it", () => {
  assert.deepEqual(boardMove("industry", "SaaS"), { industry: "SaaS" })
  assert.deepEqual(boardMove("industry", ""), { industry: null })
})

test("a card title is the name, then the id", () => {
  assert.equal(cardTitle({ id: "a", data: { name: "Acme" } }), "Acme")
  assert.equal(cardTitle({ id: "a", data: {} }), "a")
  assert.equal(viewNameOk("pipeline"), true)
  assert.equal(viewNameOk("Pipeline"), false)
  assert.equal(viewNameOk(""), false)
})
