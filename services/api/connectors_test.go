package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestConnectorSyncWritesRecords(t *testing.T) {
	var mu sync.Mutex
	mode := "down"
	var auth, agent string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = r.Header.Get("Authorization")
		agent = r.Header.Get("User-Agent")
		current := mode
		mu.Unlock()
		if r.Method != http.MethodGet {
			http.Error(w, "nope", http.StatusBadRequest)
			return
		}
		switch current {
		case "down":
			http.Error(w, "nope", http.StatusInternalServerError)
		case "junk":
			_, _ = io.WriteString(w, `{"company_name":"Acme"}`)
		case "one":
			_, _ = io.WriteString(w, `[{"id":"ext-1","company_name":"Acme","headcount":3,"sector":"SaaS"}]`)
		default:
			_, _ = io.WriteString(w, `[{"id":"ext-1","company_name":"Acme Inc","headcount":3,"sector":"SaaS"},{"id":"ext-2","company_name":"North","headcount":8,"sector":"Finance"}]`)
		}
	}))
	defer remote.Close()

	token := newOwner(t, "Connectors", "connectors-owner@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `
			delete from jobs where organization_id = $1::uuid and kind = 'connector'
		`, orgID)
	})
	postObject(t, token, companyObject())
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "connectors-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec := apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "stripe", "label": "Stripe", "kind": "api_key", "secret": "sk_test_123",
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/connectors", member, map[string]any{
		"name": "contacts", "label": "Contacts", "credential": "stripe", "object": "company",
		"url": remote.URL, "external_id": "id", "mapping": map[string]string{"company_name": "name"},
	})
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodPost, "/api/connectors", token, map[string]any{
		"name": "Bad-Name", "label": "Contacts", "credential": "stripe", "object": "company",
		"url": remote.URL, "external_id": "id", "mapping": map[string]string{"company_name": "name"},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid name" {
		t.Fatalf("name %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/connectors", token, map[string]any{
		"name": "contacts", "label": "Contacts", "credential": "stripe", "object": "company",
		"url": "ftp://example.com/rows", "external_id": "id", "mapping": map[string]string{"company_name": "name"},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid url" {
		t.Fatalf("url %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/connectors", token, map[string]any{
		"name": "contacts", "label": "Contacts", "credential": "missing", "object": "company",
		"url": remote.URL, "external_id": "id", "mapping": map[string]string{"company_name": "name"},
	})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPost, "/api/connectors", token, map[string]any{
		"name": "contacts", "label": "Contacts", "credential": "stripe", "object": "company",
		"url": remote.URL, "external_id": "id", "mapping": map[string]string{"company_name": "missing"},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown field missing" {
		t.Fatalf("field %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/connectors", token, map[string]any{
		"name": "contacts", "label": "Contacts", "credential": "stripe", "object": "company",
		"url": remote.URL, "external_id": "id",
		"mapping": map[string]string{"company_name": "name", "headcount": "employees", "sector": "industry"},
	})
	requireStatus(t, rec, http.StatusCreated)
	if rec.Body.String() != "" && containsSecret(rec.Body.String()) {
		t.Fatalf("create leaked %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/connectors", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if containsSecret(rec.Body.String()) {
		t.Fatalf("list leaked %s", rec.Body.String())
	}
	other := newOwner(t, "Connectors Other", "connectors-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/connectors/contacts", other, nil)
	requireStatus(t, rec, http.StatusNotFound)

	rec = apiDo(t, http.MethodPost, "/api/connectors/contacts/sync", token, nil)
	requireStatus(t, rec, http.StatusAccepted)
	var accepted struct {
		Event string `json:"event"`
	}
	mustJSON(t, rec, &accepted)
	if len(listRecords(t, token, "company", "")) != 0 {
		t.Fatal("the sync request called the url")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed := listJobs(t, token, "event="+accepted.Event)
	if len(failed) != 1 || failed[0].Kind != "connector" || failed[0].Status != "pending" || failed[0].Attempts != 1 || failed[0].LastError == nil || *failed[0].LastError != "connector status 500" {
		t.Fatalf("failed %+v", failed)
	}
	if len(listRecords(t, token, "company", "")) != 0 {
		t.Fatal("a failed sync stored a record")
	}

	mu.Lock()
	mode = "junk"
	mu.Unlock()
	releaseConnectorJobs(t, orgID)
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed = listJobs(t, token, "event="+accepted.Event)
	if len(failed) != 1 || failed[0].Attempts != 2 || failed[0].LastError == nil || *failed[0].LastError != "invalid connector response" {
		t.Fatalf("junk %+v", failed)
	}

	mu.Lock()
	mode = "one"
	mu.Unlock()
	releaseConnectorJobs(t, orgID)
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if auth != "Bearer sk_test_123" || agent != "Openforce-Connector" {
		t.Fatalf("auth %s agent %s", auth, agent)
	}
	mu.Unlock()
	rows := listRecords(t, token, "company", "")
	if len(rows) != 1 || rows[0].Data["name"] != "Acme" || rows[0].Data["industry"] != "SaaS" {
		t.Fatalf("rows %+v", rows)
	}
	employees, ok := asFloat(rows[0].Data["employees"])
	if !ok || employees != 3 {
		t.Fatalf("employees %+v", rows[0].Data["employees"])
	}
	created := eventsNamed(eventsFor(listEvents(t, token, "record="+rows[0].ID), rows[0].ID), "record.created")
	if len(created) != 1 || created[0].Source != "Integration" || created[0].ActorID != userOf(t, token) {
		t.Fatalf("event %+v", created)
	}
	done := listJobs(t, token, "event="+accepted.Event)
	if len(done) != 1 || done[0].Status != "done" || done[0].LastError != nil {
		t.Fatalf("done %+v", done)
	}

	mu.Lock()
	mode = "two"
	mu.Unlock()
	rec = apiDo(t, http.MethodPost, "/api/connectors/contacts/sync", token, nil)
	requireStatus(t, rec, http.StatusAccepted)
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows = listRecords(t, token, "company", "")
	if len(rows) != 2 {
		t.Fatalf("second sync rows %+v", rows)
	}
	var sawUpdate, sawNorth bool
	for _, row := range rows {
		if row.Data["name"] == "Acme Inc" && row.Version == 2 {
			sawUpdate = true
		}
		if row.Data["name"] == "North" && row.Data["industry"] == "Finance" {
			sawNorth = true
		}
	}
	if !sawUpdate || !sawNorth {
		t.Fatalf("rows %+v", rows)
	}
	if n, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil || n != 0 {
		t.Fatalf("follow-up %d %v", n, err)
	}

	rec = apiDo(t, http.MethodDelete, "/api/connectors/contacts", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodPost, "/api/connectors/contacts/sync", token, nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func containsSecret(body string) bool {
	return len(body) > 0 && (contains(body, "sk_test_123"))
}

func contains(body, part string) bool {
	return len(part) > 0 && len(body) >= len(part) && (body == part || len(body) > 0 && stringIndex(body, part) >= 0)
}

func stringIndex(body, part string) int {
	return len([]byte(body[:])) - len([]byte(body[:])) + indexOf(body, part)
}

func indexOf(body, part string) int {
	for i := 0; i+len(part) <= len(body); i++ {
		if body[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

func releaseConnectorJobs(t *testing.T, orgID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `
		update jobs
		set run_at = now()
		where organization_id = $1::uuid and kind = 'connector' and status = 'pending'
	`, orgID); err != nil {
		t.Fatal(err)
	}
}
