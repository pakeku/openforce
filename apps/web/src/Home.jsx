import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { api } from "./api.js"
import { objectTablePath } from "./routes.js"
import { Actions, Button, List, ListItem, Meta, Notice, Page, Panel } from "./ui.jsx"

function titleFor(label, rows) {
  if (!rows) return label
  return `${label} (${rows.length})`
}

export function Home({ token }) {
  const navigate = useNavigate()
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
      <div className="row g-3">
        <div className="col-12 col-lg-4">
          <Panel title={titleFor("Approvals", approvals)}>
            <Actions>
              <Button type="button" onClick={() => navigate("/approvals")}>Approvals</Button>
            </Actions>
            <Notice tone="error">{approvalError}</Notice>
            {approvals && approvals.length === 0 && <Notice>No approvals waiting.</Notice>}
            <List>
              {(approvals || []).slice(0, 3).map((row) => (
                <ListItem key={row.id} title={row.name} meta={<Meta>{row.created_at}</Meta>} />
              ))}
            </List>
          </Panel>
        </div>
        <div className="col-12 col-lg-4">
          <Panel title={titleFor("Notifications", notifications)}>
            <Actions>
              <Button type="button" onClick={() => navigate("/notifications")}>Notifications</Button>
            </Actions>
            <Notice tone="error">{notificationError}</Notice>
            {notifications && notifications.length === 0 && <Notice>No notifications.</Notice>}
            <List>
              {(notifications || []).slice(0, 3).map((row) => (
                <ListItem
                  key={row.id}
                  title={row.title}
                  meta={<><Meta>{row.body}</Meta><Meta>{row.created_at}</Meta></>}
                />
              ))}
            </List>
          </Panel>
        </div>
        <div className="col-12 col-lg-4">
          <Panel title={titleFor("Objects", objects)}>
            <Notice tone="error">{objectError}</Notice>
            {objects && objects.length === 0 && <Notice>No objects yet.</Notice>}
            <List>
              {(objects || []).slice(0, 3).map((object) => (
                <ListItem
                  key={object.id}
                  title={object.label}
                  meta={<Meta>{object.name}</Meta>}
                  actions={<Button type="button" variant="secondary" onClick={() => navigate(objectTablePath(object.name))}>Open</Button>}
                />
              ))}
            </List>
          </Panel>
        </div>
      </div>
    </Page>
  )
}
