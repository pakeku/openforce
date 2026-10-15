import assert from "node:assert/strict"
import test from "node:test"
import { calendarMove, cardsOnDay, monthLabel, monthWeeks, recordDay, shiftMonth, unscheduledCards, weekdays } from "./calendar.js"

test("a month is six Monday-first weeks", () => {
  const weeks = monthWeeks(2026, 10)
  assert.equal(weeks.length, 6)
  assert.equal(weeks.every((week) => week.length === 7), true)
  assert.equal(new Date(`${weeks[0][0].date}T00:00:00Z`).getUTCDay(), 1)
  const inMonth = weeks.flat().filter((day) => day.inMonth)
  assert.equal(inMonth.length, 31)
  assert.equal(inMonth[0].date, "2026-10-01")
  assert.equal(inMonth[30].date, "2026-10-31")
  assert.deepEqual(weekdays, ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"])
  assert.equal(monthLabel(2026, 10), "October 2026")
})

test("the month cursor crosses years", () => {
  assert.deepEqual(shiftMonth(2026, 12, 1), { year: 2027, month: 1 })
  assert.deepEqual(shiftMonth(2026, 1, -1), { year: 2025, month: 12 })
})

test("a card sits on its date or stays unscheduled", () => {
  const records = [
    { id: "a", data: { name: "Acme", renews: "2026-10-15" } },
    { id: "b", data: { name: "Beta", renews: "2026-10-15T18:00:00Z" } },
    { id: "c", data: { name: "Blank" } },
    { id: "d", data: { renews: "nope" } },
  ]
  assert.equal(recordDay(records[0], "renews"), "2026-10-15")
  assert.equal(recordDay(records[1], "renews"), "2026-10-15")
  assert.deepEqual(cardsOnDay(records, "renews", "2026-10-15").map((record) => record.id), ["a", "b"])
  assert.deepEqual(unscheduledCards(records, "renews").map((record) => record.id), ["c", "d"])
  assert.deepEqual(calendarMove("renews", "2026-10-20"), { renews: "2026-10-20" })
  assert.deepEqual(calendarMove("renews", ""), { renews: null })
})
