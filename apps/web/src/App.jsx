import { useState } from "react"
import { BrowserRouter, NavLink, Navigate, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom"
import { api } from "./api.js"
import { Approvals } from "./Approvals.jsx"
import { Flows } from "./Flows.jsx"
import { Home } from "./Home.jsx"
import { Notifications } from "./Notifications.jsx"
import { Objects } from "./Objects.jsx"
import { RecordPage, TablePage } from "./Records.jsx"
import { isWide, shellPath } from "./routes.js"
import { Button, Field, Notice, Panel } from "./ui.jsx"

const links = [
  ["home", "/home", "Home"],
  ["objects", "/objects", "Objects"],
  ["flows", "/flows", "Flows"],
  ["approvals", "/approvals", "Approvals"],
  ["notifications", "/notifications", "Notifications"],
]

export default function App() {
  return (
    <BrowserRouter>
      <Shell />
    </BrowserRouter>
  )
}

function Shell() {
  const location = useLocation()
  const navigate = useNavigate()
  const [token, setToken] = useState(() => sessionStorage.getItem("openforce.token") || "")
  const [draft, setDraft] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const section = shellPath(location.pathname)

  function enter(next) {
    setToken(next)
    setPassword("")
    setDraft("")
    setError("")
    const path = section === "home" && location.pathname === "/" ? "/home" : location.pathname
    navigate(path === "/" ? "/home" : path)
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
        enter(next)
      })
      .catch((err) => setError(err.message))
  }

  function saveToken(event) {
    event.preventDefault()
    const next = draft.trim()
    if (next === "") return
    sessionStorage.setItem("openforce.token", next)
    enter(next)
  }

  function signOut() {
    sessionStorage.removeItem("openforce.token")
    setToken("")
    setPassword("")
    navigate("/home")
  }

  return (
    <div className="min-vh-100">
      <nav className="navbar navbar-expand-lg bg-white border-bottom sticky-top">
        <div className="container-xl">
          <NavLink className="navbar-brand" to="/home">Openforce</NavLink>
          {token && (
            <>
              <button className="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#openforce-nav" aria-controls="openforce-nav" aria-expanded="false" aria-label="Open navigation">
                <span className="navbar-toggler-icon" />
              </button>
              <div className="collapse navbar-collapse" id="openforce-nav">
                <ul className="navbar-nav me-auto">
                  {links.map(([name, path, label]) => (
                    <li className="nav-item" key={name}>
                      <NavLink
                        to={path}
                        end={name !== "objects"}
                        className={({ isActive }) => `nav-link${isActive ? " active" : ""}`}
                      >
                        {label}
                      </NavLink>
                    </li>
                  ))}
                </ul>
                <Button variant="text" type="button" onClick={signOut}>Sign out</Button>
              </div>
            </>
          )}
        </div>
      </nav>
      <main className={isWide(location.pathname) ? "container-fluid px-3 px-lg-4 py-4" : "container-xl py-4"}>
        {token === "" ? (
          <div className="row justify-content-center g-4">
            <div className="col-12 col-lg-5">
              <Panel title="Sign in" onSubmit={signIn}>
                <Field label="Email">
                  <input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" required />
                </Field>
                <Field label="Password">
                  <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required />
                </Field>
                <Notice tone="error">{error}</Notice>
                <Button>Sign in</Button>
              </Panel>
            </div>
            <div className="col-12 col-lg-5">
              <Panel title="Use a token" onSubmit={saveToken}>
                <Field label="Bearer token">
                  <input value={draft} onChange={(event) => setDraft(event.target.value)} autoComplete="off" />
                </Field>
                <Button variant="secondary">Use token</Button>
              </Panel>
            </div>
          </div>
        ) : (
          <Routes>
            <Route path="/" element={<Navigate to="/home" replace />} />
            <Route path="/home" element={<Home token={token} />} />
            <Route path="/objects" element={<Objects token={token} />} />
            <Route path="/objects/:name/new" element={<RecordRoute token={token} />} />
            <Route path="/objects/:name/:id" element={<RecordRoute token={token} />} />
            <Route path="/objects/:name" element={<TableRoute token={token} />} />
            <Route path="/flows" element={<Flows token={token} />} />
            <Route path="/approvals" element={<Approvals token={token} />} />
            <Route path="/notifications" element={<Notifications token={token} />} />
            <Route path="*" element={<Navigate to="/home" replace />} />
          </Routes>
        )}
      </main>
    </div>
  )
}

function TableRoute({ token }) {
  const { name } = useParams()
  return <TablePage token={token} object={name} />
}

function RecordRoute({ token }) {
  const { name, id } = useParams()
  return <RecordPage token={token} object={name} id={id} />
}
