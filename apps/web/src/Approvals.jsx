import { useEffect, useState } from "react"
import { api } from "./api.js"
import { Actions, Button, Notice, Page } from "./ui.jsx"

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
    <Page title="Approvals">
      <Notice tone="error">{error}</Notice>
      {rows.length === 0 && <Notice>No approvals waiting.</Notice>}
      <ul className="approval-list">
        {rows.map((row) => (
          <li key={row.id} className="approval-row">
            <div>
              <strong>{row.name}</strong>
              <span>{row.created_at}</span>
            </div>
            <Actions>
              <Button type="button" onClick={() => decide(row.id, "approve")}>Approve</Button>
              <Button type="button" variant="secondary" onClick={() => decide(row.id, "reject")}>Reject</Button>
            </Actions>
          </li>
        ))}
      </ul>
    </Page>
  )
}
