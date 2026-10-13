package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type server struct {
	pool *pgxpool.Pool
}

func newMux(pool *pgxpool.Pool) http.Handler {
	s := &server{pool: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(pool.Ping))
	mux.HandleFunc("POST /api/meta/objects", s.requireAuth(s.createObject))
	mux.HandleFunc("GET /api/meta/objects", s.requireAuth(s.listObjects))
	mux.HandleFunc("GET /api/meta/objects/{name}", s.requireAuth(s.getObject))
	mux.HandleFunc("POST /api/meta/objects/{name}/relationships", s.requireAuth(s.createRelationship))
	mux.HandleFunc("PUT /api/meta/objects/{name}/permissions", s.requireAuth(s.putPermissions))
	mux.HandleFunc("GET /api/objects/{name}", s.requireAuth(s.listRecords))
	mux.HandleFunc("POST /api/objects/{name}", s.requireAuth(s.createRecord))
	mux.HandleFunc("GET /api/objects/{name}/{id}", s.requireAuth(s.getRecord))
	mux.HandleFunc("PATCH /api/objects/{name}/{id}", s.requireAuth(s.patchRecord))
	mux.HandleFunc("DELETE /api/objects/{name}/{id}", s.requireAuth(s.deleteRecord))
	mux.HandleFunc("GET /api/objects/{name}/{id}/related/{rel}", s.requireAuth(s.listRelated))
	mux.HandleFunc("POST /api/objects/{name}/{id}/related/{rel}", s.requireAuth(s.linkRelated))
	return mux
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
