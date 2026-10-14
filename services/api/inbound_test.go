package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInboundHookCreatesARecord(t *testing.T) {
	token := newOwner(t, "Inbound", "inbound-owner@example.com")
	postObject(t, token, companyObject())
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgOf(t, token), "Mia", "inbound-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec := apiDo(t, http.MethodPost, "/api/inbound", member, map[string]any{"name": "leads", "object": "company"})
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodPost, "/api/inbound", token, map[string]any{"name": "Bad-Name", "object": "company"})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid name" {
		t.Fatalf("name %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/inbound", token, map[string]any{"name": "leads", "object": "missing"})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodGet, "/api/inbound", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)

	rec = apiDo(t, http.MethodPost, "/api/inbound", token, map[string]any{"name": "leads", "object": "company"})
	requireStatus(t, rec, http.StatusCreated)
	var created inboundView
	mustJSON(t, rec, &created)
	if created.ID == "" || created.Secret == "" || created.Name != "leads" || created.Object != "company" {
		t.Fatalf("created %+v", created)
	}
	rec = apiDo(t, http.MethodPost, "/api/inbound", token, map[string]any{"name": "leads", "object": "company"})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "inbound already exists" {
		t.Fatalf("duplicate %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/inbound", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if bytes.Contains(rec.Body.Bytes(), []byte(created.Secret)) {
		t.Fatalf("list returned a secret %s", rec.Body.String())
	}
	other := newOwner(t, "Inbound Other", "inbound-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/inbound", other, nil)
	requireStatus(t, rec, http.StatusOK)
	if bytes.Contains(rec.Body.Bytes(), []byte(created.ID)) {
		t.Fatalf("other org saw %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodDelete, "/api/inbound/leads", other, nil)
	requireStatus(t, rec, http.StatusNotFound)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "stamp_in", "label": "Stamp", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "update", "data": map[string]any{"industry": "Finance"}}},
	})
	requireStatus(t, rec, http.StatusCreated)

	body := []byte(`{"employees":1}`)
	rec = postHook(t, created.ID, signWebhook(created.Secret, body), body)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "missing required field name" {
		t.Fatalf("field %s", rec.Body.String())
	}
	rec = postHook(t, created.ID, "sha256=nope", []byte(`{"name":"Acme"}`))
	requireStatus(t, rec, http.StatusUnauthorized)
	rec = postHook(t, created.ID, signWebhook(created.Secret, []byte("not-json")), []byte("not-json"))
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid json" {
		t.Fatalf("json %s", rec.Body.String())
	}
	if len(listRecords(t, token, "company", "")) != 0 {
		t.Fatal("a rejected hook stored a record")
	}

	payload, err := json.Marshal(map[string]any{"name": "Acme", "employees": 3, "industry": "SaaS"})
	if err != nil {
		t.Fatal(err)
	}
	rec = postHook(t, created.ID, signWebhook(created.Secret, payload), payload)
	requireStatus(t, rec, http.StatusCreated)
	var record recordView
	mustJSON(t, rec, &record)
	if record.Data["name"] != "Acme" || record.Data["industry"] != "SaaS" {
		t.Fatalf("record %+v", record)
	}
	got := getRecord(t, token, "company", record.ID)
	if got.Data["industry"] != "SaaS" {
		t.Fatal("the hook ran the workflow")
	}
	createdEvents := eventsNamed(eventsFor(listEvents(t, token, "record="+record.ID), record.ID), "record.created")
	if len(createdEvents) != 1 || createdEvents[0].Source != "Integration" || createdEvents[0].ActorID != userOf(t, token) {
		t.Fatalf("event %+v", createdEvents)
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	got = getRecord(t, token, "company", record.ID)
	if got.Data["industry"] != "Finance" || got.Version != 2 {
		t.Fatalf("after work %+v", got)
	}
	ran := listExecutions(t, token, "stamp_in")
	if len(ran) != 1 || ran[0].Status != "done" {
		t.Fatalf("execution %+v", ran)
	}
	var sawAutomation bool
	for _, ev := range eventsFor(listEvents(t, token, "record="+record.ID), record.ID) {
		if ev.Name == "record.updated" && ev.Source == "Automation" {
			sawAutomation = true
		}
	}
	if !sawAutomation {
		t.Fatal("workflow did not update the inbound record")
	}
	if n, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil || n != 0 {
		t.Fatalf("second delivery %d %v", n, err)
	}

	rec = apiDo(t, http.MethodDelete, "/api/inbound/leads", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = postHook(t, created.ID, signWebhook(created.Secret, payload), payload)
	requireStatus(t, rec, http.StatusNotFound)
	if len(listRecords(t, token, "company", "")) != 1 {
		t.Fatal("a deleted hook stored another record")
	}
}

func postHook(t *testing.T, id, signature string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/"+id, bytes.NewReader(body))
	if signature != "" {
		req.Header.Set("X-Openforce-Signature", signature)
	}
	rec := httptest.NewRecorder()
	newMux(testPool).ServeHTTP(rec, req)
	return rec
}
