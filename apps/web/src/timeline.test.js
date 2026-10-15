import assert from "node:assert/strict"
import test from "node:test"
import { barIndexes, monthDays, recordSpan, shiftSpan, timelineMove } from "./timeline.js"

const october = monthDays(2026, 10)

test("a month lists each of its dates", () => {
  assert.equal(october.length, 31)
  assert.equal(october[0], "2026-10-01")
  assert.equal(october[30], "2026-10-31")
  assert.equal(monthDays(2026, 2).length, 28)
})

test("a bar covers the span and clips to the month", () => {
  const records = [
    { id: "a", data: { starts: "2026-10-02", ends: "2026-10-06" } },
    { id: "b", data: { starts: "2026-09-28", ends: "2026-10-03" } },
    { id: "c", data: { starts: "2026-11-02", ends: "2026-11-04" } },
    { id: "d", data: { starts: "2026-10-15" } },
    { id: "e", data: { ends: "2026-10-04" } },
  ]
  assert.deepEqual(recordSpan(records[0], "starts", "ends"), { start: "2026-10-02", end: "2026-10-06" })
  assert.deepEqual(barIndexes("2026-10-02", "2026-10-06", october), { from: 1, to: 5 })
  assert.deepEqual(barIndexes("2026-09-28", "2026-10-03", october), { from: 0, to: 2 })
  assert.equal(barIndexes("2026-11-02", "2026-11-04", october), null)
  assert.deepEqual(barIndexes("2026-10-15", "", october), { from: 14, to: 14 })
  assert.deepEqual(barIndexes("2026-10-06", "2026-10-02", october), { from: 1, to: 5 })
  assert.equal(recordSpan(records[4], "starts", "ends").start, "")
})

test("moving a bar keeps its length", () => {
  assert.deepEqual(shiftSpan("2026-10-02", "2026-10-06", "2026-10-10"), { start: "2026-10-10", end: "2026-10-14" })
  assert.deepEqual(shiftSpan("2026-10-15", "", "2026-10-18"), { start: "2026-10-18", end: "" })
  assert.deepEqual(shiftSpan("2026-10-02", "2026-10-06", ""), { start: "", end: "2026-10-06" })
  assert.deepEqual(timelineMove("starts", "ends", "2026-10-10", "2026-10-14"), {
    starts: "2026-10-10",
    ends: "2026-10-14",
  })
  assert.deepEqual(timelineMove("starts", "ends", "", "2026-10-06"), { starts: null, ends: "2026-10-06" })
})
