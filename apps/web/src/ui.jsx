import { Children, cloneElement } from "react"

const buttonClass = {
  primary: "btn btn-primary",
  secondary: "btn btn-outline-secondary",
  text: "btn btn-link",
  brand: "btn btn-link fw-semibold text-body text-decoration-none",
}

export function Button({ variant = "primary", className = "", ...props }) {
  const names = [buttonClass[variant] || buttonClass.primary, className].filter(Boolean).join(" ")
  return <button className={names} {...props} />
}

export function Field({ label, children }) {
  const child = Children.only(children)
  const isCheck = child.props.type === "checkbox" || child.props.type === "radio" || child.props.field?.type === "boolean"
  const controlClass = isCheck ? "form-check-input" : child.type === "select" ? "form-select" : "form-control"
  const control = typeof child.type === "string"
    ? cloneElement(child, { className: [controlClass, child.props.className].filter(Boolean).join(" ") })
    : child
  if (isCheck) {
    return (
      <label className="form-check mb-3">
        {control}
        <span className="form-check-label">{label}</span>
      </label>
    )
  }
  return (
    <label className="form-label d-block mb-3">
      {label}
      {control}
    </label>
  )
}

export function Notice({ tone = "empty", children }) {
  if (children == null || children === false || children === "") return null
  if (tone === "error") return <div className="alert alert-danger" role="alert">{children}</div>
  return <p className="text-secondary mb-3">{children}</p>
}

export function Panel({ title, children, onSubmit }) {
  const body = (
    <div className="card-body">
      {title ? <h2 className="h5 card-title">{title}</h2> : null}
      {children}
    </div>
  )
  if (onSubmit) return <form className="card mb-3" onSubmit={onSubmit}>{body}</form>
  return <section className="card mb-3">{body}</section>
}

export function Page({ title, actions, children }) {
  return (
    <>
      {(title || actions) && (
        <div className="d-flex flex-wrap justify-content-between align-items-center gap-2 mb-3">
          {title ? <h1 className="h3 mb-0">{title}</h1> : <span />}
          {actions ? <div className="d-flex flex-wrap gap-2 align-items-center">{actions}</div> : null}
        </div>
      )}
      {children}
    </>
  )
}

export function Actions({ children }) {
  return <div className="d-flex flex-wrap gap-2 align-items-center">{children}</div>
}

export function List({ children }) {
  return <div className="list-group mb-3">{children}</div>
}

export function ListButton({ children, className = "", ...props }) {
  return (
    <button type="button" className={`list-group-item list-group-item-action d-flex justify-content-between align-items-center gap-3 text-start ${className}`.trim()} {...props}>
      {children}
    </button>
  )
}

export function ListItem({ title, meta, actions }) {
  return (
    <div className="list-group-item d-flex flex-wrap justify-content-between align-items-center gap-3">
      <div>
        <div className="fw-semibold">{title}</div>
        {meta}
      </div>
      {actions}
    </div>
  )
}

export function Meta({ children }) {
  return <div className="small text-secondary">{children}</div>
}
