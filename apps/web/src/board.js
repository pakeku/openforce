const viewNamePattern = /^[a-z][a-z0-9_]*$/

export function viewNameOk(name) {
  return viewNamePattern.test(String(name || "").trim())
}

export function boardColumns(options, records, group) {
  const lanes = (options || []).filter((option) => option != null && option !== "").map((option) => ({
    key: String(option),
    label: String(option),
  }))
  const known = new Set(lanes.map((lane) => lane.key))
  const stray = (records || []).some((record) => !known.has(record?.data?.[group]))
  if (stray) lanes.push({ key: "", label: "None" })
  return lanes
}

export function cardColumn(record, group, options) {
  const value = record?.data?.[group]
  if ((options || []).map((option) => String(option)).includes(value)) return value
  return ""
}

export function cardsInColumn(records, group, column, options) {
  return (records || []).filter((record) => cardColumn(record, group, options) === column)
}

export function boardMove(group, column) {
  return { [group]: column === "" ? null : column }
}

export function cardTitle(record) {
  const name = record?.data?.name
  if (name !== undefined && name !== null && String(name) !== "") return String(name)
  return record?.id || ""
}
