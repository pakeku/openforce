import { useState } from "react"
import { api } from "./api.js"
import { Approvals } from "./Approvals.jsx"
import { Flows } from "./Flows.jsx"
import { Objects } from "./Objects.jsx"
import { RecordPage, TablePage } from "./Records.jsx"

export default function App() {
  const [token, setToken] = useState(() => sessionStorage.getItem("openforce.token") || "")
  const [draft, setDraft] = useState(token)
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [screen, setScreen] = useState({ page: "objects" })

  function saveToken(event) {
    event.preventDefault()
    const next = draft.trim()
    sessionStorage.setItem("openforce.token", next)
    setToken(next)
    setError("")
    setScreen({ page: "objects" })
  }

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
        setDraft(next)
        setPassword("")
        setScreen({ page: "objects" })
      })
      .catch((err) => setError(err.message))
  }

  const section = screen.page === "flows" ? "flows" : screen.page === "approvals" ? "approvals" : "objects"

  return (
    <div className={section === "flows" ? "app wide" : "app"}>
      <header>
        <div className="brand-row">
          <button className="brand" onClick={() => setScreen({ page: "objects" })}>Openforce</button>
          {token && (
            <nav>
              <button className="text" aria-current={section === "objects" ? "page" : undefined} onClick={() => setScreen({ page: "objects" })}>Objects</button>
              <button className="text" aria-current={section === "flows" ? "page" : undefined} onClick={() => setScreen({ page: "flows" })}>Flows</button>
              <button className="text" aria-current={section === "approvals" ? "page" : undefined} onClick={() => setScreen({ page: "approvals" })}>Approvals</button>
            </nav>
          )}
        </div>
        <div className="header-auth">
          {token === "" && (
            <form onSubmit={signIn}>
              <input
                className="compact"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="Email"
                aria-label="Email"
                autoComplete="username"
              />
              <input
                className="compact"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="Password"
                aria-label="Password"
                autoComplete="current-password"
              />
              <button>Sign in</button>
            </form>
          )}
          <form onSubmit={saveToken}>
            <input
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder="Bearer token"
              aria-label="Bearer token"
            />
            <button>Use token</button>
          </form>
          {error !== "" && <p className="error">{error}</p>}
        </div>
      </header>
      {token === "" ? (
        <p className="empty">Sign in, or paste the token printed by the seed command.</p>
      ) : screen.page === "flows" ? (
        <Flows token={token} />
      ) : screen.page === "approvals" ? (
        <Approvals token={token} />
      ) : screen.page === "table" ? (
        <TablePage token={token} object={screen.object} open={setScreen} />
      ) : screen.page === "record" ? (
        <RecordPage token={token} object={screen.object} id={screen.id} open={setScreen} />
      ) : (
        <Objects token={token} open={setScreen} />
      )}
    </div>
  )
}
