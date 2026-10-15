import { useEffect, useState } from "react"
import { api } from "./api.js"
import { Actions, Button, Notice, Page, Panel } from "./ui.jsx"

function titleFor(label, rows) {
  if (!rows) return label
  return `${label} (${rows.length})`
}

export function Home({ token, open }) {
  const [approvals, setApprovals] = useState(null)
  const [approvalError, setApprovalError] = useState("")
  const [notifications, setNotifications] = useState(null)
  const [notificationError, setNotificationError] = useState("")
  const [objects, setObjects] = useState(null)
  const [objectError, setObjectError] = useState("")

  useEffect(() => {
    let gone = false
    api(token, "GET", "/api/approvals")
      .then((body) => {
        if (!gone) setApprovals(body?.approvals || [])
      })
      .catch((err) => {
        if (!gone) setApprovalError(err.message)
      })
    api(token, "GET", "/api/notifications")
      .then((body) => {
        if (!gone) setNotifications(body?.notifications || [])
      })
      .catch((err) => {
        if (!gone) setNotificationError(err.message)
      })
    api(token, "GET", "/api/meta/objects")
      .then((body) => {
        if (!gone) setObjects(body?.objects || [])
      })
      .catch((err) => {
        if (!gone) setObjectError(err.message)
      })
    return () => { gone = true }
  }, [token])

  return (
    <Page title="Home">
      <div className="home">
        <Panel title={titleFor("Approvals", approvals)}>
          <Actions>
            <Button type="button" onClick={() => open({ page: "approvals" })}>Approvals</Button>
          </Actions>
          <Notice tone="error">{approvalError}</Notice>
          {approvals && approvals.length === 0 && <Notice>No approvals waiting.</Notice>}
          <ul className="approval-list">
            {(approvals || []).slice(0, 3).map((row) => (
              <li key={row.id} className="approval-row">
                <div>
                  <strong>{row.name}</strong>
                  <span>{row.created_at}</span>
                </div>
              </li>
            ))}
          </ul>
        </Panel>
        <Panel title={titleFor("Notifications", notifications)}>
          <Actions>
            <Button type="button" onClick={() => open({ page: "notifications" })}>Notifications</Button>
          </Actions>
          <Notice tone="error">{notificationError}</Notice>
          {notifications && notifications.length === 0 && <Notice>No notifications.</Notice>}
          <ul className="notification-list">
            {(notifications || []).slice(0, 3).map((row) => (
              <li key={row.id} className="notification-row">
                <div>
                  <strong>{row.title}</strong>
                  <span>{row.body}</span>
                  <span>{row.created_at}</span>
                </div>
              </li>
            ))}
          </ul>
        </Panel>
        <Panel title={titleFor("Objects", objects)}>
          <Notice tone="error">{objectError}</Notice>
          {objects && objects.length === 0 && <Notice>No objects yet.</Notice>}
          <ul className="object-list">
            {(objects || []).slice(0, 3).map((object) => (
              <li key={object.id}>
                <Button type="button" onClick={() => open({ page: "table", object: object.name })}>
                  <strong>{object.label}</strong>
                  <span>{object.name}</span>
                </Button>
              </li>
            ))}
          </ul>
        </Panel>
      </div>
    </Page>
  )
}
