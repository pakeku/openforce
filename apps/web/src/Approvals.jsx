import { useEffect, useState } from "react"
import { api } from "./api.js"
import { Actions, Button, List, ListItem, Meta, Notice, Page } from "./ui.jsx"

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
      <List>
        {rows.map((row) => (
          <ListItem
            key={row.id}
            title={row.name}
            meta={<Meta>{row.created_at}</Meta>}
            actions={(
              <Actions>
                <Button type="button" onClick={() => decide(row.id, "approve")}>Approve</Button>
                <Button type="button" variant="secondary" onClick={() => decide(row.id, "reject")}>Reject</Button>
              </Actions>
            )}
          />
        ))}
      </List>
    </Page>
  )
}
