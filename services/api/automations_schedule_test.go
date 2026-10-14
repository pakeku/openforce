package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestAutomationRunsOnASchedule(t *testing.T) {
	var mu sync.Mutex
	var hits int
	var body []byte
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits++
		body = raw
		mu.Unlock()
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer bad.Close()

	token := newOwner(t, "Schedules", "schedules-owner@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `
			delete from automation_definitions
			where organization_id = $1::uuid and name in ('digest', 'ping')
		`, orgID)
	})

	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "soon", "label": "Soon", "every": "1s",
		"actions": []any{map[string]any{"type": "http", "url": "https://example.com/hook"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid schedule" {
		t.Fatalf("short %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "both", "label": "Both", "event": "record.created", "object": "note", "every": "1m",
		"actions": []any{map[string]any{"type": "create", "object": "note", "data": map[string]any{"body": "x"}}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid schedule" {
		t.Fatalf("both %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "ruled", "label": "Ruled", "every": "1m",
		"conditions": []any{map[string]any{"field": "body", "op": "eq", "value": "x"}},
		"actions":    []any{map[string]any{"type": "http", "url": "https://example.com/hook"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid schedule" {
		t.Fatalf("conditions %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "edit", "label": "Edit", "every": "1m",
		"actions": []any{map[string]any{"type": "update", "data": map[string]any{"body": "x"}}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("update %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "self", "label": "Self", "every": "1m",
		"actions": []any{map[string]any{
			"type": "http", "url": "https://example.com/hook",
			"data": map[string]any{"id": map[string]any{"record": true}},
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("record %s", rec.Body.String())
	}

	postObject(t, token, map[string]any{
		"name":  "note",
		"label": "Note",
		"fields": []map[string]any{
			{"name": "body", "type": "text", "required": true},
		},
	})
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "digest", "label": "Digest", "every": "1m",
		"actions": []any{map[string]any{"type": "create", "object": "note", "data": map[string]any{"body": "tick"}}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var created automationView
	mustJSON(t, rec, &created)
	if created.Every != "1m" || created.Event != "" || created.NextRun == nil || !created.NextRun.After(time.Now()) {
		t.Fatalf("created %+v", created)
	}
	rec = apiDo(t, http.MethodPatch, "/api/automations/digest", token, map[string]any{
		"version": 1, "label": "Hourly",
	})
	requireStatus(t, rec, http.StatusOK)
	var patched automationView
	mustJSON(t, rec, &patched)
	if patched.Label != "Hourly" || patched.Every != "1m" || patched.Version != 2 {
		t.Fatalf("patched %+v", patched)
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "ping", "label": "Ping", "every": "1m",
		"actions": []any{map[string]any{"type": "http", "url": bad.URL}},
	})
	requireStatus(t, rec, http.StatusCreated)

	if n, err := (&server{pool: testPool}).enqueueDueSchedules(context.Background()); err != nil || n != 0 {
		t.Fatalf("early enqueue %d %v", n, err)
	}
	if len(listRecords(t, token, "note", "")) != 0 {
		t.Fatal("a future schedule created a record")
	}

	if _, err := testPool.Exec(context.Background(), `
		update automation_definitions
		set next_run_at = now() - interval '1 second'
		where organization_id = $1::uuid and name in ('digest', 'ping')
	`, orgID); err != nil {
		t.Fatal(err)
	}
	if n, err := (&server{pool: testPool}).enqueueDueSchedules(context.Background()); err != nil || n != 2 {
		t.Fatalf("enqueue %d %v", n, err)
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}

	notes := listRecords(t, token, "note", "")
	if len(notes) != 1 || notes[0].Data["body"] != "tick" {
		t.Fatalf("notes %+v", notes)
	}
	ran := listExecutions(t, token, "digest")
	if len(ran) != 1 || ran[0].Status != "done" || len(ran[0].Created) != 1 || ran[0].Created[0] != notes[0].ID {
		t.Fatalf("digest %+v", ran)
	}
	noteEvents := eventsNamed(eventsFor(listEvents(t, token, "record="+notes[0].ID), notes[0].ID), "record.created")
	if len(noteEvents) != 1 || len(listJobs(t, token, "event="+noteEvents[0].ID)) != 0 {
		t.Fatal("schedule retriggered on the record it created")
	}

	ticks := listEvents(t, token, "name=schedule.tick")
	if len(ticks) != 2 {
		t.Fatalf("ticks %+v", ticks)
	}
	var pingEvent string
	for _, ev := range ticks {
		if ev.ActorID != userOf(t, token) || ev.Source != "API" || ev.RecordID != nil {
			t.Fatalf("tick %+v", ev)
		}
		if ev.Payload["automation"] == "ping" {
			pingEvent = ev.ID
		}
	}
	if pingEvent == "" {
		t.Fatal("missing ping tick")
	}
	mu.Lock()
	wrapped := append([]byte(nil), body...)
	seen := hits
	mu.Unlock()
	if seen != 1 {
		t.Fatalf("calls %d", seen)
	}
	var posted map[string]any
	if err := json.Unmarshal(wrapped, &posted); err != nil {
		t.Fatal(err)
	}
	if posted["event"] != "schedule.tick" || posted["record_id"] != "" {
		t.Fatalf("body %s", wrapped)
	}
	if len(listExecutions(t, token, "ping")) != 0 {
		t.Fatal("a failed call recorded an execution")
	}
	pending := listJobs(t, token, "event="+pingEvent+"&status=pending")
	if len(pending) != 1 || pending[0].Attempts != 1 || pending[0].LastError == nil || *pending[0].LastError != "http status 500" {
		t.Fatalf("pending %+v", pending)
	}

	if n, err := (&server{pool: testPool}).enqueueDueSchedules(context.Background()); err != nil || n != 0 {
		t.Fatalf("second enqueue %d %v", n, err)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/ping", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listJobs(t, token, "event="+pingEvent)) != 0 {
		t.Fatal("deleted workflow left its job")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(listRecords(t, token, "note", "")) != 1 {
		t.Fatal("the schedule ran again before it was due")
	}
}
