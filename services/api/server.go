package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type server struct {
	pool  *pgxpool.Pool
	redis *redis.Client
}

func newMux(pool *pgxpool.Pool) http.Handler {
	s := &server{pool: pool, redis: sharedRedis}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(pool.Ping))
	mux.HandleFunc("GET /api/events", s.requireAuth(s.listEvents))
	mux.HandleFunc("GET /api/jobs", s.requireUser(s.listJobs))
	mux.HandleFunc("GET /api/webhooks", s.requireUser(s.listWebhooks))
	mux.HandleFunc("POST /api/webhooks", s.requireUser(s.createWebhook))
	mux.HandleFunc("DELETE /api/webhooks/{id}", s.requireUser(s.deleteWebhook))
	mux.HandleFunc("GET /api/inbound", s.requireUser(s.listInbound))
	mux.HandleFunc("POST /api/inbound", s.requireUser(s.createInbound))
	mux.HandleFunc("DELETE /api/inbound/{name}", s.requireUser(s.deleteInbound))
	mux.HandleFunc("POST /api/hooks/{id}", s.acceptInbound)
	mux.HandleFunc("GET /api/credentials", s.requireUser(s.listCredentials))
	mux.HandleFunc("POST /api/credentials", s.requireUser(s.createCredential))
	mux.HandleFunc("GET /api/credentials/{name}", s.requireUser(s.getCredential))
	mux.HandleFunc("PATCH /api/credentials/{name}", s.requireUser(s.patchCredential))
	mux.HandleFunc("DELETE /api/credentials/{name}", s.requireUser(s.deleteCredential))
	mux.HandleFunc("POST /api/credentials/{name}/refresh", s.requireUser(s.refreshCredential))
	mux.HandleFunc("GET /api/connectors", s.requireUser(s.listConnectors))
	mux.HandleFunc("POST /api/connectors", s.requireUser(s.createConnector))
	mux.HandleFunc("GET /api/connectors/{name}", s.requireUser(s.getConnector))
	mux.HandleFunc("DELETE /api/connectors/{name}", s.requireUser(s.deleteConnector))
	mux.HandleFunc("POST /api/connectors/{name}/sync", s.requireUser(s.requestSync))
	mux.HandleFunc("GET /api/keys", s.requireUser(s.listAPIKeys))
	mux.HandleFunc("POST /api/keys", s.requireUser(s.createAPIKey))
	mux.HandleFunc("GET /api/keys/{name}", s.requireUser(s.getAPIKey))
	mux.HandleFunc("PATCH /api/keys/{name}", s.requireUser(s.patchAPIKey))
	mux.HandleFunc("DELETE /api/keys/{name}", s.requireUser(s.deleteAPIKey))
	mux.HandleFunc("GET /api/functions", s.requireUser(s.listFunctions))
	mux.HandleFunc("POST /api/functions", s.requireUser(s.createFunction))
	mux.HandleFunc("DELETE /api/functions/{name}", s.requireUser(s.deleteFunction))
	mux.HandleFunc("GET /api/automations", s.requireUser(s.listAutomations))
	mux.HandleFunc("POST /api/automations", s.requireUser(s.createAutomation))
	mux.HandleFunc("GET /api/automations/{name}", s.requireUser(s.getAutomation))
	mux.HandleFunc("PATCH /api/automations/{name}", s.requireUser(s.patchAutomation))
	mux.HandleFunc("DELETE /api/automations/{name}", s.requireUser(s.deleteAutomation))
	mux.HandleFunc("GET /api/automations/{name}/executions", s.requireUser(s.listExecutions))
	mux.HandleFunc("POST /api/meta/objects", s.requireUser(s.createObject))
	mux.HandleFunc("GET /api/meta/objects", s.requireUser(s.listObjects))
	mux.HandleFunc("GET /api/meta/objects/{name}", s.requireUser(s.getObject))
	mux.HandleFunc("GET /api/meta/objects/{name}/relationships", s.requireUser(s.listRelationships))
	mux.HandleFunc("POST /api/meta/objects/{name}/relationships", s.requireUser(s.createRelationship))
	mux.HandleFunc("PUT /api/meta/objects/{name}/permissions", s.requireUser(s.putPermissions))
	mux.HandleFunc("GET /api/meta/objects/{name}/views", s.requireUser(s.listViews))
	mux.HandleFunc("POST /api/meta/objects/{name}/views", s.requireUser(s.createView))
	mux.HandleFunc("GET /api/meta/objects/{name}/views/{view}", s.requireUser(s.getView))
	mux.HandleFunc("PATCH /api/meta/objects/{name}/views/{view}", s.requireUser(s.patchView))
	mux.HandleFunc("GET /api/meta/objects/{name}/layouts", s.requireUser(s.listLayouts))
	mux.HandleFunc("POST /api/meta/objects/{name}/layouts", s.requireUser(s.createLayout))
	mux.HandleFunc("GET /api/meta/objects/{name}/layouts/{layout}", s.requireUser(s.getLayout))
	mux.HandleFunc("PATCH /api/meta/objects/{name}/layouts/{layout}", s.requireUser(s.patchLayout))
	mux.HandleFunc("GET /api/meta/objects/{name}/pages", s.requireUser(s.listPages))
	mux.HandleFunc("POST /api/meta/objects/{name}/pages", s.requireUser(s.createPage))
	mux.HandleFunc("GET /api/meta/objects/{name}/pages/{page}", s.requireUser(s.getPage))
	mux.HandleFunc("PATCH /api/meta/objects/{name}/pages/{page}", s.requireUser(s.patchPage))
	mux.HandleFunc("GET /api/openapi.json", s.requireAuth(s.openAPI))
	mux.HandleFunc("GET /api/objects/{name}", s.requireAuth(s.listRecords))
	mux.HandleFunc("POST /api/objects/{name}", s.requireAuth(s.createRecord))
	mux.HandleFunc("GET /api/objects/{name}/{id}", s.requireAuth(s.getRecord))
	mux.HandleFunc("GET /api/objects/{name}/{id}/audit", s.requireAuth(s.listAudit))
	mux.HandleFunc("PATCH /api/objects/{name}/{id}", s.requireAuth(s.patchRecord))
	mux.HandleFunc("DELETE /api/objects/{name}/{id}", s.requireAuth(s.deleteRecord))
	mux.HandleFunc("GET /api/objects/{name}/{id}/related/{rel}", s.requireAuth(s.listRelated))
	mux.HandleFunc("POST /api/objects/{name}/{id}/related/{rel}", s.requireAuth(s.linkRelated))
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Openforce-Signature")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Println(err)
	}
}

func writeErr(w http.ResponseWriter, err error) {
	var api *apiError
	if errors.As(err, &api) {
		writeJSON(w, api.status, map[string]string{"error": api.msg})
		return
	}
	log.Println(err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
}

func readJSON(r *http.Request, dest any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return apiErr(http.StatusBadRequest, "invalid json")
	}
	return nil
}
