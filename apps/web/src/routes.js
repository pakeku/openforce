const sections = ["home", "objects", "flows", "approvals", "notifications"]

export function shellPath(pathname) {
  const name = String(pathname || "").split("/").filter(Boolean)[0]
  return sections.includes(name) ? name : "home"
}

export function isWide(pathname) {
  return shellPath(pathname) === "flows"
}

export function objectTablePath(name) {
  return `/objects/${encodeURIComponent(name)}`
}

export function objectFieldsPath(name) {
  return `/objects/${encodeURIComponent(name)}/fields`
}

export function objectRecordPath(name, id, query) {
  const path = id
    ? `/objects/${encodeURIComponent(name)}/${encodeURIComponent(id)}`
    : `/objects/${encodeURIComponent(name)}/new`
  if (!query) return path
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== null && value !== "") params.set(key, String(value))
  }
  const text = params.toString()
  return text ? `${path}?${text}` : path
}
