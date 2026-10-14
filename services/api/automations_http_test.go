package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestAutomationCallsHTTP(t *testing.T) {
	var mu sync.Mutex
	type hit struct {
		method      string
		contentType string
		userAgent   string
		signature   string
		body        []byte
	}
	var sent []hit
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = append(sent, hit{r.Method, r.Header.Get("Content-Type"), r.Header.Get("User-Agent"), r.Header.Get("X-Openforce-Signature"), body})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer okServer.Close()

	var failed []hit
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		failed = append(failed, hit{r.Method, r.Header.Get("Content-Type"), r.Header.Get("User-Agent"), r.Header.Get("X-Openforce-Signature"), body})
		mu.Unlock()
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer badServer.Close()

	token := newOwner(t, "HTTP", "http-owner@example.com")
	postObject(t, token, companyObject())
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `
			delete from automation_definitions
			where organization_id = $1::uuid and name = 'alert'
		`, orgID)
	})

	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "bad_url", "label": "Bad url", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "http", "url": "ftp://example.com/hook"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid url" {
		t.Fatalf("url %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "bad_field", "label": "Bad field", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{
			"type": "http", "url": okServer.URL,
			"data": map[string]any{"notes": []any{map[string]any{"field": "missing"}}},
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown field missing" {
		t.Fatalf("field %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "notify", "label": "Notify", "event": "record.created", "object": "company",
		"conditions": []any{map[string]any{"field": "employees", "op": "gte", "value": 1}},
		"actions": []any{map[string]any{
			"type": "http", "url": okServer.URL,
			"data": map[string]any{
				"name":    map[string]any{"field": "name"},
				"company": map[string]any{"record": true},
				"notes":   []any{map[string]any{"field": "industry"}},
			},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "alert", "label": "Alert", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "http", "url": badServer.URL}},
	})
	requireStatus(t, rec, http.StatusCreated)

	company := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 3, "industry": "SaaS"})
	mu.Lock()
	early := len(sent) + len(failed)
	mu.Unlock()
	if early != 0 {
		t.Fatal("the record request called the url")
	}
	var createdEvent string
	for _, ev := range eventsFor(listEvents(t, token, "record="+company.ID), company.ID) {
		if ev.Name == "record.created" {
			createdEvent = ev.ID
		}
	}
	if createdEvent == "" {
		t.Fatal("missing record.created")
	}
	if len(listJobs(t, token, "event="+createdEvent)) != 2 {
		t.Fatalf("jobs %+v", listJobs(t, token, "event="+createdEvent))
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	sentCopy := append([]hit(nil), sent...)
	failedCopy := append([]hit(nil), failed...)
	mu.Unlock()
	if len(sentCopy) != 1 || sentCopy[0].method != http.MethodPost || sentCopy[0].contentType != "application/json" || sentCopy[0].userAgent != "Openforce-Action" || sentCopy[0].signature != "" {
		t.Fatalf("sent %+v", sentCopy)
	}
	var posted map[string]any
	if err := json.Unmarshal(sentCopy[0].body, &posted); err != nil {
		t.Fatal(err)
	}
	notes, _ := posted["notes"].([]any)
	if posted["name"] != "Acme" || posted["company"] != company.ID || len(notes) != 1 || notes[0] != "SaaS" {
		t.Fatalf("body %s", sentCopy[0].body)
	}
	if len(failedCopy) != 1 || failedCopy[0].method != http.MethodPost || failedCopy[0].userAgent != "Openforce-Action" {
		t.Fatalf("failed %+v", failedCopy)
	}
	var wrapped map[string]any
	if err := json.Unmarshal(failedCopy[0].body, &wrapped); err != nil {
		t.Fatal(err)
	}
	data, _ := wrapped["data"].(map[string]any)
	if wrapped["event"] != "record.created" || wrapped["object"] != "company" || wrapped["record_id"] != company.ID || data["name"] != "Acme" {
		t.Fatalf("wrapped %s", failedCopy[0].body)
	}
	ran := listExecutions(t, token, "notify")
	if len(ran) != 1 || ran[0].Status != "done" || len(ran[0].Created) != 0 {
		t.Fatalf("notify %+v", ran)
	}
	if len(listExecutions(t, token, "alert")) != 0 {
		t.Fatal("a failed call recorded an execution")
	}
	pending := listJobs(t, token, "event="+createdEvent+"&status=pending")
	if len(pending) != 1 || pending[0].Kind != "automation" || pending[0].Attempts != 1 || pending[0].LastError == nil || *pending[0].LastError != "http status 500" {
		t.Fatalf("pending %+v", pending)
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	pending = listJobs(t, token, "event="+createdEvent+"&status=pending")
	if len(pending) != 1 || pending[0].Attempts != 1 {
		t.Fatalf("retried before the wait %+v", pending)
	}
	mu.Lock()
	if len(sent) != 1 || len(failed) != 1 {
		t.Fatalf("calls sent %d failed %d", len(sent), len(failed))
	}
	mu.Unlock()

	rec = apiDo(t, http.MethodDelete, "/api/automations/alert", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listJobs(t, token, "event="+createdEvent+"&status=pending")) != 0 {
		t.Fatal("deleted workflow left its job")
	}
	if _, err := testPool.Exec(context.Background(), `
		update jobs
		set status = 'pending', run_at = now()
		where event_id = $1::uuid and kind = 'automation' and status = 'done'
	`, createdEvent); err != nil {
		t.Fatal(err)
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(sent) != 1 {
		t.Fatalf("retry called the url again, sent %d", len(sent))
	}
	mu.Unlock()
	if len(listJobs(t, token, "event="+createdEvent+"&status=done")) != 1 {
		t.Fatal("replay did not finish")
	}

	postRecord(t, token, "company", map[string]any{"name": "Quiet", "employees": 0})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(sent) != 1 || len(failed) != 1 {
		t.Fatalf("skipped event called sent %d failed %d", len(sent), len(failed))
	}
	mu.Unlock()
	var doneN, skippedN int
	for _, execution := range listExecutions(t, token, "notify") {
		switch execution.Status {
		case "done":
			doneN++
		case "skipped":
			skippedN++
		}
	}
	if doneN != 1 || skippedN != 1 {
		t.Fatalf("executions done %d skipped %d", doneN, skippedN)
	}
}
