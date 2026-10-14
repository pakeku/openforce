package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestWebhookDelivery(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var signatures []string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		signatures = append(signatures, r.Header.Get("X-Openforce-Signature"))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	var failBody []byte
	var failSig string
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		failBody = body
		failSig = r.Header.Get("X-Openforce-Signature")
		mu.Unlock()
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer failing.Close()

	token := newOwner(t, "Webhooks", "webhook-owner@example.com")
	postObject(t, token, companyObject())
	rec := apiDo(t, http.MethodPost, "/api/webhooks", token, map[string]any{
		"url": "ftp://example.com/hook", "events": []string{"record.created"},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid url" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/webhooks", token, map[string]any{
		"url": receiver.URL, "events": []string{},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "events are required" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/webhooks", token, map[string]any{
		"url": receiver.URL, "events": []string{"nope"},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown event" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/webhooks", token, map[string]any{
		"url": receiver.URL, "events": []string{"record.created", "record.created"},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "duplicate event" {
		t.Fatalf("body %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/webhooks", token, map[string]any{
		"url": receiver.URL, "events": []string{"record.created", "record.deleted"},
	})
	requireStatus(t, rec, http.StatusCreated)
	var created webhookView
	mustJSON(t, rec, &created)
	if created.Secret == "" || created.URL != receiver.URL || len(created.Events) != 2 {
		t.Fatalf("created %+v", created)
	}
	rec = apiDo(t, http.MethodPost, "/api/webhooks", token, map[string]any{
		"url": receiver.URL, "events": []string{"record.updated"},
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "webhook already exists" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/webhooks", token, map[string]any{
		"url": failing.URL, "events": []string{"record.updated"},
	})
	requireStatus(t, rec, http.StatusCreated)
	var failingHook webhookView
	mustJSON(t, rec, &failingHook)

	rec = apiDo(t, http.MethodGet, "/api/webhooks", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), created.Secret) || strings.Contains(rec.Body.String(), failingHook.Secret) {
		t.Fatalf("list returned a secret %s", rec.Body.String())
	}
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgOf(t, token), "Mia", "webhook-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodPost, "/api/webhooks", member, map[string]any{
		"url": "https://example.com/member", "events": []string{"record.created"},
	})
	requireStatus(t, rec, http.StatusForbidden)
	other := newOwner(t, "Webhooks Other", "webhook-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/webhooks", other, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), receiver.URL) {
		t.Fatalf("other org saw %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodDelete, "/api/webhooks/"+created.ID, other, nil)
	requireStatus(t, rec, http.StatusNotFound)

	acme := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	recordEvents := eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	var createdEvent, fieldEvent string
	for _, ev := range recordEvents {
		if ev.Name == "record.created" {
			createdEvent = ev.ID
		}
		if ev.Name == "field.changed" && ev.Payload["field"] == "name" {
			fieldEvent = ev.ID
		}
	}
	if createdEvent == "" || fieldEvent == "" {
		t.Fatalf("events %s", namesOf(recordEvents))
	}
	if kindsOf(listJobs(t, token, "event="+createdEvent)) != "webhook" {
		t.Fatalf("create jobs %s", kindsOf(listJobs(t, token, "event="+createdEvent)))
	}
	if kindsOf(listJobs(t, token, "event="+fieldEvent)) != "audit" {
		t.Fatalf("field jobs %s", kindsOf(listJobs(t, token, "event="+fieldEvent)))
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(bodies) != 1 || signatures[0] != signWebhook(created.Secret, bodies[0]) {
		t.Fatalf("deliveries %d signature %v", len(bodies), signatures)
	}
	var delivered webhookEvent
	if err := json.Unmarshal(bodies[0], &delivered); err != nil {
		t.Fatal(err)
	}
	mu.Unlock()
	if delivered.Name != "record.created" || delivered.OrganizationID != orgOf(t, token) || delivered.Payload["object"] != "company" {
		t.Fatalf("delivered %+v", delivered)
	}

	rec = apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": 1,
		"data":    map[string]any{"name": "Acme Inc"},
	})
	requireStatus(t, rec, http.StatusOK)
	recordEvents = eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	var updatedEvent string
	for _, ev := range recordEvents {
		if ev.Name == "record.updated" {
			updatedEvent = ev.ID
		}
	}
	if updatedEvent == "" {
		t.Fatal("missing record.updated")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed := listJobs(t, token, "event="+updatedEvent)
	if len(failed) != 1 || failed[0].Kind != "webhook" || failed[0].Status != "pending" || failed[0].Attempts != 1 || failed[0].LastError == nil || *failed[0].LastError != "webhook status 500" {
		t.Fatalf("failed job %+v", failed)
	}
	mu.Lock()
	if failSig != signWebhook(failingHook.Secret, failBody) || len(bodies) != 1 {
		t.Fatalf("fail signature %s deliveries %d", failSig, len(bodies))
	}
	mu.Unlock()
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed = listJobs(t, token, "event="+updatedEvent)
	if failed[0].Attempts != 1 || failed[0].Status != "pending" {
		t.Fatalf("retried before the wait %+v", failed[0])
	}

	rec = apiDo(t, http.MethodDelete, "/api/webhooks/"+created.ID, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodDelete, "/api/objects/company/"+acme.ID, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	recordEvents = eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	var deletedEvent string
	for _, ev := range recordEvents {
		if ev.Name == "record.deleted" {
			deletedEvent = ev.ID
		}
	}
	if kindsOf(listJobs(t, token, "event="+deletedEvent)) != "audit" {
		t.Fatalf("deleted jobs %s", kindsOf(listJobs(t, token, "event="+deletedEvent)))
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("deleted subscription still delivered %d", len(bodies))
	}
}

func kindsOf(jobs []jobView) string {
	names := make([]string, len(jobs))
	for i, job := range jobs {
		names[i] = job.Kind
	}
	return strings.Join(names, ",")
}
