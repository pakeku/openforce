import { useEffect, useState } from "react"
import { api } from "./api.js"
import { Notice, Page } from "./ui.jsx"

export function Notifications({ token }) {
  const [rows, setRows] = useState([])
  const [error, setError] = useState("")

  async function reload() {
    const body = await api(token, "GET", "/api/notifications")
    setRows(body.notifications || [])
  }

  useEffect(() => {
    let gone = false
    reload().catch((err) => {
      if (!gone) setError(err.message)
    })
    return () => { gone = true }
  }, [token])

  return (
    <Page title="Notifications">
      <Notice tone="error">{error}</Notice>
      {rows.length === 0 && <Notice>No notifications.</Notice>}
      <ul className="notification-list">
        {rows.map((row) => (
          <li key={row.id} className="notification-row">
            <div>
              <strong>{row.title}</strong>
              <span>{row.body}</span>
              <span>{row.created_at}</span>
            </div>
          </li>
        ))}
      </ul>
    </Page>
  )
}
