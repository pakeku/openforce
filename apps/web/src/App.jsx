import { useState } from "react"
import { Flows } from "./Flows.jsx"
import { Objects } from "./Objects.jsx"
import { RecordPage, TablePage } from "./Records.jsx"

export default function App() {
  const [token, setToken] = useState(() => sessionStorage.getItem("openforce.token") || "")
  const [draft, setDraft] = useState(token)
  const [screen, setScreen] = useState({ page: "objects" })

  function saveToken(event) {
    event.preventDefault()
    const next = draft.trim()
    sessionStorage.setItem("openforce.token", next)
    setToken(next)
    setScreen({ page: "objects" })
  }

  const section = screen.page === "flows" ? "flows" : "objects"

  return (
    <div className={section === "flows" ? "app wide" : "app"}>
      <header>
        <div className="brand-row">
          <button className="brand" onClick={() => setScreen({ page: "objects" })}>Openforce</button>
          {token && (
            <nav>
              <button className="text" aria-current={section === "objects" ? "page" : undefined} onClick={() => setScreen({ page: "objects" })}>Objects</button>
              <button className="text" aria-current={section === "flows" ? "page" : undefined} onClick={() => setScreen({ page: "flows" })}>Flows</button>
            </nav>
          )}
        </div>
        <form onSubmit={saveToken}>
          <input
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="Bearer token"
            aria-label="Bearer token"
          />
          <button>Use token</button>
        </form>
      </header>
      {token === "" ? (
        <p className="empty">Paste the token printed by the seed command.</p>
      ) : screen.page === "flows" ? (
        <Flows token={token} />
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
