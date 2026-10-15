const namePattern = /^[a-z][a-z0-9_]*$/

export const fieldTypeOptions = ["text", "number", "currency", "date", "boolean", "enum", "link"]

export function fieldPayload(field) {
  const type = field.type === "link" ? "relation" : field.type
  const body = {
    name: String(field.name || "").trim(),
    type,
    required: Boolean(field.required),
  }
  if (type === "enum") {
    const options = Array.isArray(field.options) ? field.options : String(field.options || "").split(",")
    body.options = options.map((item) => String(item).trim()).filter(Boolean)
  }
  if (type === "relation") body.to = String(field.to || "").trim()
  return body
}

export function fieldDraft(field) {
  return {
    name: field?.name || "",
    type: field?.type === "relation" ? "link" : (field?.type || "text"),
    required: Boolean(field?.required),
    options: Array.isArray(field?.options) ? field.options.join(", ") : "",
    to: field?.to || "",
  }
}

export function linkTargets(objects, name, label) {
  const targets = (objects || []).map((object) => ({
    name: object.name,
    label: object.label || object.name,
  }))
  const next = String(name || "").trim()
  if (namePattern.test(next) && !targets.some((object) => object.name === next)) {
    targets.unshift({ name: next, label: String(label || "").trim() || next })
  }
  return targets
}
