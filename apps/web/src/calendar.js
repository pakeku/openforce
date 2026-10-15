export const weekdays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]

export function shiftMonth(year, month, delta) {
  const next = new Date(Date.UTC(year, month - 1 + delta, 1))
  return { year: next.getUTCFullYear(), month: next.getUTCMonth() + 1 }
}

export function monthLabel(year, month) {
  return new Date(Date.UTC(year, month - 1, 1)).toLocaleString("en-US", {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  })
}

export function monthWeeks(year, month) {
  const first = new Date(Date.UTC(year, month - 1, 1))
  const startOffset = (first.getUTCDay() + 6) % 7
  const start = new Date(first)
  start.setUTCDate(1 - startOffset)
  const weeks = []
  for (let week = 0; week < 6; week += 1) {
    const days = []
    for (let day = 0; day < 7; day += 1) {
      const current = new Date(start)
      current.setUTCDate(start.getUTCDate() + week * 7 + day)
      days.push({
        date: current.toISOString().slice(0, 10),
        inMonth: current.getUTCMonth() === month - 1,
      })
    }
    weeks.push(days)
  }
  return weeks
}

export function recordDay(record, group) {
  const value = record?.data?.[group]
  if (typeof value !== "string" || value.length < 10) return ""
  const day = value.slice(0, 10)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) return ""
  const parsed = new Date(`${day}T00:00:00Z`)
  if (Number.isNaN(parsed.getTime()) || parsed.toISOString().slice(0, 10) !== day) return ""
  return day
}

export function cardsOnDay(records, group, day) {
  return (records || []).filter((record) => recordDay(record, group) === day)
}

export function unscheduledCards(records, group) {
  return (records || []).filter((record) => recordDay(record, group) === "")
}

export function calendarMove(group, day) {
  return { [group]: day === "" ? null : day }
}
