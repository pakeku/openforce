import { useEffect, useState } from "react"
import { api } from "./api.js"
import { List, ListItem, Meta, Notice, Page } from "./ui.jsx"

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
      <List>
        {rows.map((row) => (
          <ListItem
            key={row.id}
            title={row.title}
            meta={<><Meta>{row.body}</Meta><Meta>{row.created_at}</Meta></>}
          />
        ))}
      </List>
    </Page>
  )
}
