export function Button({ variant = "primary", className = "", ...props }) {
  const names = [variant === "primary" ? "" : variant, className].filter(Boolean).join(" ")
  return <button className={names || undefined} {...props} />
}

export function Field({ label, children }) {
  return (
    <label className="field">
      {label}
      {children}
    </label>
  )
}

export function Notice({ tone = "empty", children }) {
  if (children == null || children === false || children === "") return null
  return <p className={tone}>{children}</p>
}

export function Panel({ title, children, onSubmit }) {
  const body = (
    <>
      {title ? <h2>{title}</h2> : null}
      {children}
    </>
  )
  if (onSubmit) {
    return <form className="panel" onSubmit={onSubmit}>{body}</form>
  }
  return <section className="panel">{body}</section>
}

export function Page({ title, actions, children }) {
  return (
    <main>
      {(title || actions) && (
        <div className="title-row">
          {title ? <h1>{title}</h1> : <span />}
          {actions}
        </div>
      )}
      {children}
    </main>
  )
}

export function Actions({ children }) {
  return <div className="actions">{children}</div>
}
