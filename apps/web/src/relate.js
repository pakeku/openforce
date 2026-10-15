export function relationshipRequests(form) {
  const name = form.name.trim()
  const inverse = form.inverse.trim()
  const from = form.from
  const to = form.to
  if (form.kind === "subobjects") {
    const field = (form.field || from).trim()
    return [
      { object: to, body: { name: field, kind: "belongs_to", to: from, inverse: name } },
      { object: from, body: { name, kind: "has_many", to, inverse: field } },
    ]
  }
  if (form.kind === "links") {
    return [{ object: from, body: { name, kind: "many_to_many", to, inverse } }]
  }
  const body = { name, kind: "belongs_to", to }
  if (inverse) body.inverse = inverse
  return [{ object: from, body }]
}

export function isChildList(group, fields) {
  if (!group?.to || !group.inverse) return false
  if (group.kind === "has_many") return true
  if (group.kind !== "belongs_to") return false
  return !(fields || []).some((field) => field.name === group.name && field.type === "relation")
}

export function isLinkList(group) {
  return group?.kind === "many_to_many"
}

export function dedupeRelationships(list) {
  const rank = { has_many: 3, many_to_many: 2, belongs_to: 1 }
  const sorted = [...(list || [])].sort((a, b) => (rank[b.kind] || 0) - (rank[a.kind] || 0))
  const seen = new Set()
  const unique = []
  for (const item of sorted) {
    if (!item?.name || seen.has(item.name)) continue
    seen.add(item.name)
    unique.push(item)
  }
  return unique.sort((a, b) => a.name.localeCompare(b.name))
}

export function safeReturnPath(value) {
  if (typeof value !== "string" || !value.startsWith("/objects/")) return ""
  if (value.startsWith("//") || value.includes("\\") || value.includes("://")) return ""
  return value
}
