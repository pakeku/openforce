package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthReturnsOKWhenDatabaseAnswers(t *testing.T) {
	handler := healthHandler(func(context.Context) error { return nil })
	status, body := getHealth(t, handler)
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200", status)
	}
	if body["status"] != "ok" {
		t.Fatalf("body %#v, want status ok", body)
	}
}

func TestHealthFailsWhenDatabaseIsDown(t *testing.T) {
	handler := healthHandler(func(context.Context) error {
		return errors.New("connection refused")
	})
	status, body := getHealth(t, handler)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", status)
	}
	if body["status"] != "unavailable" {
		t.Fatalf("body %#v, want status unavailable", body)
	}
}

func getHealth(t *testing.T, handler http.HandlerFunc) (int, map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}
