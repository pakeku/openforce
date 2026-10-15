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

export function objectRecordPath(name, id) {
  if (id) return `/objects/${encodeURIComponent(name)}/${encodeURIComponent(id)}`
  return `/objects/${encodeURIComponent(name)}/new`
}
