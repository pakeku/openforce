import { recordDay } from "./calendar.js"

export function monthDays(year, month) {
  const count = new Date(Date.UTC(year, month, 0)).getUTCDate()
  const days = []
  for (let day = 1; day <= count; day += 1) {
    days.push(new Date(Date.UTC(year, month - 1, day)).toISOString().slice(0, 10))
  }
  return days
}

export function recordSpan(record, startField, endField) {
  return {
    start: recordDay(record, startField),
    end: recordDay(record, endField),
  }
}

export function addDays(day, count) {
  const date = new Date(`${day}T00:00:00Z`)
  date.setUTCDate(date.getUTCDate() + count)
  return date.toISOString().slice(0, 10)
}

export function dayDistance(start, end) {
  const from = new Date(`${start}T00:00:00Z`).getTime()
  const to = new Date(`${end}T00:00:00Z`).getTime()
  return Math.round((to - from) / 86400000)
}

export function shiftSpan(start, end, newStart) {
  if (!newStart) return { start: "", end: end || "" }
  if (!start || !end) return { start: newStart, end: end || "" }
  return { start: newStart, end: addDays(newStart, dayDistance(start, end)) }
}

export function barIndexes(start, end, days) {
  if (!start || !days?.length) return null
  const finish = end || start
  const [fromDay, toDay] = start <= finish ? [start, finish] : [finish, start]
  const first = days[0]
  const last = days[days.length - 1]
  if (toDay < first || fromDay > last) return null
  const from = fromDay < first ? 0 : days.indexOf(fromDay)
  const to = toDay > last ? days.length - 1 : days.indexOf(toDay)
  if (from < 0 || to < 0 || to < from) return null
  return { from, to }
}

export function timelineMove(startField, endField, start, end) {
  return {
    [startField]: start === "" ? null : start,
    [endField]: end === "" ? null : end,
  }
}
