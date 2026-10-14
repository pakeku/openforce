import { useState } from "react"
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

  return (
    <div className="app">
      <header>
        <button className="brand" onClick={() => setScreen({ page: "objects" })}>Openforce</button>
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
