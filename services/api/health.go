package main

import (
	"context"
	"encoding/json"
	"net/http"
)

type probe func(context.Context) error

func healthHandler(ping probe) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		body := "ok"
		if err := ping(r.Context()); err != nil {
			status = http.StatusServiceUnavailable
			body = "unavailable"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": body})
	}
}
