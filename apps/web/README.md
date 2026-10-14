# Openforce web

The UI runtime. It reads object definitions, views, layouts, and pages from the kernel API and renders a table and a record form.

```text
npm install
npm run dev
```

Open http://localhost:5173 and paste the token printed by `go run -C services/api . seed`. The API defaults to http://localhost:8080. Set `VITE_API_URL` when it listens somewhere else.
