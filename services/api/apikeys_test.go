package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAPIKeyDoesNotInheritTheOwner(t *testing.T) {
	token := newOwner(t, "Keys", "keys-owner@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `delete from jobs where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name": "note", "label": "Note",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "copy_note", "label": "Copy", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{
			"type": "create", "object": "note", "data": map[string]any{"name": "Copied"},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "keys-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodPost, "/api/keys", member, map[string]any{
		"name": "writer", "label": "Writer", "grants": []any{},
	})
	requireStatus(t, rec, http.StatusForbidden)

	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "Bad-Name", "label": "Writer", "grants": []any{},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid name" {
		t.Fatalf("name %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "writer", "label": "", "grants": []any{},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "label is required" {
		t.Fatalf("label %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "writer", "label": "Writer",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "grants are required" {
		t.Fatalf("grants %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "writer", "label": "Writer",
		"grants": []any{map[string]any{"object": "missing", "can_read": true, "can_write": true}},
	})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "writer", "label": "Writer",
		"grants": []any{
			map[string]any{"object": "company", "can_read": true, "can_write": true},
			map[string]any{"object": "company", "can_read": true, "can_write": false},
		},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "duplicate object" {
		t.Fatalf("duplicate %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "writer", "label": "Writer",
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": true}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var writer apiKeyView
	mustJSON(t, rec, &writer)
	if writer.Token == "" || writer.Name != "writer" || len(writer.Grants) != 1 || !writer.Grants[0].CanWrite {
		t.Fatalf("writer %+v", writer)
	}
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "reader", "label": "Reader",
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": false}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var reader apiKeyView
	mustJSON(t, rec, &reader)
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "empty", "label": "Empty", "grants": []any{},
	})
	requireStatus(t, rec, http.StatusCreated)
	var empty apiKeyView
	mustJSON(t, rec, &empty)
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "writer", "label": "Writer", "grants": []any{},
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "api key already exists" {
		t.Fatalf("duplicate key %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodGet, "/api/keys", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), writer.Token) || strings.Contains(rec.Body.String(), reader.Token) {
		t.Fatalf("list leaked %s", rec.Body.String())
	}
	var listed struct {
		Keys []apiKeyView `json:"keys"`
	}
	mustJSON(t, rec, &listed)
	if len(listed.Keys) != 3 {
		t.Fatalf("list %+v", listed.Keys)
	}
	other := newOwner(t, "Keys Other", "keys-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/keys/writer", other, nil)
	requireStatus(t, rec, http.StatusNotFound)

	rec = apiDo(t, http.MethodGet, "/api/credentials", writer.Token, nil)
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodGet, "/api/jobs", writer.Token, nil)
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodPost, "/api/meta/objects", writer.Token, map[string]any{
		"name": "secret", "label": "Secret",
		"fields": []map[string]any{{"name": "name", "type": "text"}},
	})
	requireStatus(t, rec, http.StatusForbidden)

	rec = apiDo(t, http.MethodPost, "/api/objects/company", reader.Token, map[string]any{"name": "Blocked"})
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodGet, "/api/objects/company", empty.Token, nil)
	requireStatus(t, rec, http.StatusForbidden)

	rec = apiDo(t, http.MethodPost, "/api/objects/company", writer.Token, map[string]any{"name": "Acme"})
	requireStatus(t, rec, http.StatusCreated)
	var row recordView
	mustJSON(t, rec, &row)
	var serviceUser string
	if err := testPool.QueryRow(context.Background(), `
		select user_id::text from api_keys
		where organization_id = $1::uuid and name = 'writer'
	`, orgID).Scan(&serviceUser); err != nil {
		t.Fatal(err)
	}
	created := eventsNamed(eventsFor(listEvents(t, token, "record="+row.ID), row.ID), "record.created")
	if len(created) != 1 || created[0].ActorID != serviceUser || created[0].ActorID == userOf(t, token) || created[0].Source != "API" {
		t.Fatalf("event %+v owner %s key %s", created, userOf(t, token), serviceUser)
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+row.ID, reader.Token, nil)
	requireStatus(t, rec, http.StatusOK)
	if len(listEvents(t, empty.Token, "record="+row.ID)) != 0 {
		t.Fatal("a key without a grant saw the event")
	}
	if len(eventsNamed(listEvents(t, reader.Token, ""), "user.created")) != 0 {
		t.Fatal("a key saw a catalog event")
	}
	if len(eventsNamed(listEvents(t, reader.Token, "record="+row.ID), "record.created")) != 1 {
		t.Fatal("a read grant missed the event")
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(listRecords(t, token, "note", "")) != 0 {
		t.Fatal("the workflow ran with the owner's rights")
	}
	pending := listJobs(t, token, "status=pending")
	sawForbidden := false
	for _, job := range pending {
		if job.Kind == "automation" && job.Attempts == 1 && job.LastError != nil && *job.LastError == "forbidden" {
			sawForbidden = true
		}
	}
	if !sawForbidden {
		t.Fatalf("workflow jobs %+v", pending)
	}

	rec = apiDo(t, http.MethodPatch, "/api/keys/reader", token, map[string]any{
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": true}},
	})
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), reader.Token) {
		t.Fatalf("patch leaked %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/objects/company", reader.Token, map[string]any{"name": "North"})
	requireStatus(t, rec, http.StatusCreated)

	rec = apiDo(t, http.MethodDelete, "/api/keys/writer", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+row.ID, writer.Token, nil)
	requireStatus(t, rec, http.StatusUnauthorized)
}
