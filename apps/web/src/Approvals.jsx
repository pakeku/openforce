import { useEffect, useState } from "react"
import { api } from "./api.js"

export function Approvals({ token }) {
  const [rows, setRows] = useState([])
  const [error, setError] = useState("")

  async function reload() {
    const body = await api(token, "GET", "/api/approvals")
    setRows(body.approvals || [])
  }

  useEffect(() => {
    let gone = false
    reload().catch((err) => {
      if (!gone) setError(err.message)
    })
    return () => { gone = true }
  }, [token])

  async function decide(id, decision) {
    setError("")
    try {
      await api(token, "POST", `/api/approvals/${id}`, { decision })
      await reload()
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <main>
      <h1>Approvals</h1>
      {error && <p className="error">{error}</p>}
      {rows.length === 0 && <p className="empty">No approvals waiting.</p>}
      <ul className="approval-list">
        {rows.map((row) => (
          <li key={row.id} className="approval-row">
            <div>
              <strong>{row.name}</strong>
              <span>{row.created_at}</span>
            </div>
            <div className="actions">
              <button type="button" onClick={() => decide(row.id, "approve")}>Approve</button>
              <button type="button" className="secondary" onClick={() => decide(row.id, "reject")}>Reject</button>
            </div>
          </li>
        ))}
      </ul>
    </main>
  )
}
