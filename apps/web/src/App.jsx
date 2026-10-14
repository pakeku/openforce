import { useState } from "react"
import { api } from "./api.js"
import { Approvals } from "./Approvals.jsx"
import { Flows } from "./Flows.jsx"
import { Objects } from "./Objects.jsx"
import { RecordPage, TablePage } from "./Records.jsx"
import { Button, Field, Notice, Panel } from "./ui.jsx"

export default function App() {
  const [token, setToken] = useState(() => sessionStorage.getItem("openforce.token") || "")
  const [draft, setDraft] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [screen, setScreen] = useState({ page: "objects" })

  function signIn(event) {
    event.preventDefault()
    setError("")
    api("", "POST", "/api/login", { email: email.trim(), password })
      .then((data) => {
        const next = data?.token || ""
        if (next === "") {
          setError("request failed")
          return
        }
        sessionStorage.setItem("openforce.token", next)
        setToken(next)
        setPassword("")
        setScreen({ page: "objects" })
      })
      .catch((err) => setError(err.message))
  }

  function saveToken(event) {
    event.preventDefault()
    const next = draft.trim()
    if (next === "") return
    sessionStorage.setItem("openforce.token", next)
    setToken(next)
    setDraft("")
    setError("")
    setScreen({ page: "objects" })
  }

  function signOut() {
    sessionStorage.removeItem("openforce.token")
    setToken("")
    setPassword("")
    setScreen({ page: "objects" })
  }

  const section = screen.page === "flows" ? "flows" : screen.page === "approvals" ? "approvals" : "objects"

  let body
  if (token === "") {
    body = (
      <main className="gate">
        <Panel title="Sign in" onSubmit={signIn}>
          <Field label="Email">
            <input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" required />
          </Field>
          <Field label="Password">
            <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required />
          </Field>
          <Notice tone="error">{error}</Notice>
          <Button>Sign in</Button>
        </Panel>
        <Panel title="Use a token" onSubmit={saveToken}>
          <Field label="Bearer token">
            <input value={draft} onChange={(event) => setDraft(event.target.value)} autoComplete="off" />
          </Field>
          <Button variant="secondary">Use token</Button>
        </Panel>
      </main>
    )
  } else if (screen.page === "flows") {
    body = <Flows token={token} />
  } else if (screen.page === "approvals") {
    body = <Approvals token={token} />
  } else if (screen.page === "table") {
    body = <TablePage token={token} object={screen.object} open={setScreen} />
  } else if (screen.page === "record") {
    body = <RecordPage token={token} object={screen.object} id={screen.id} open={setScreen} />
  } else {
    body = <Objects token={token} open={setScreen} />
  }

  return (
    <div className={section === "flows" ? "app wide" : "app"}>
      <header className="topbar">
        <Button variant="brand" onClick={() => setScreen({ page: "objects" })}>Openforce</Button>
        {token && (
          <nav>
            <Button variant="text" aria-current={section === "objects" ? "page" : undefined} onClick={() => setScreen({ page: "objects" })}>Objects</Button>
            <Button variant="text" aria-current={section === "flows" ? "page" : undefined} onClick={() => setScreen({ page: "flows" })}>Flows</Button>
            <Button variant="text" aria-current={section === "approvals" ? "page" : undefined} onClick={() => setScreen({ page: "approvals" })}>Approvals</Button>
          </nav>
        )}
        {token && <Button variant="text" onClick={signOut}>Sign out</Button>}
      </header>
      {body}
    </div>
  )
}
