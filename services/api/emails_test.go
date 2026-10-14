package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestWorkflowStoresOutboundEmail(t *testing.T) {
	token := newOwner(t, "Email", "email-owner@example.com")
	orgID := orgOf(t, token)
	const memberEmail = "email-member@example.com"
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `delete from automation_definitions where organization_id = $1::uuid`, orgID)
		_, _ = testPool.Exec(ctx, `delete from jobs where organization_id = $1::uuid and status <> 'done'`, orgID)
	})
	postObject(t, token, map[string]any{
		"name":  "lead",
		"label": "Lead",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "email", "type": "text", "required": true},
			{"name": "note", "type": "text"},
		},
	})
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", memberEmail)
	if err != nil {
		t.Fatal(err)
	}
	other := newOwner(t, "Email Other", "email-other@example.com")

	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "no_subject", "label": "No subject", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{"type": "email", "to": "ada@example.com", "body": ""}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "subject is required" {
		t.Fatalf("subject %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "blank_subject", "label": "Blank subject", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{"type": "email", "to": "ada@example.com", "subject": "", "body": "x"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "subject is required" {
		t.Fatalf("blank subject %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "no_to", "label": "No to", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{"type": "email", "subject": "Hi", "body": ""}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "to is required" {
		t.Fatalf("to %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "record_to", "label": "Record to", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{
			"type": "email", "to": map[string]any{"record": true}, "subject": "Hi", "body": "",
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("record to %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "record_subject", "label": "Record subject", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{
			"type": "email", "to": "ada@example.com", "subject": map[string]any{"record": true}, "body": "",
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("record subject %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "record_body", "label": "Record body", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{
			"type": "email", "to": "ada@example.com", "subject": "Hi", "body": map[string]any{"record": true},
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("record body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "bad_field", "label": "Bad field", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{
			"type": "email", "to": "ada@example.com", "subject": map[string]any{"field": "missing"}, "body": "",
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown field missing" {
		t.Fatalf("field %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "clock_field", "label": "Clock field", "every": "1m",
		"actions": []any{map[string]any{
			"type": "email", "to": map[string]any{"field": "email"}, "subject": "Hi", "body": "",
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("schedule field %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "clock_record", "label": "Clock record", "every": "1m",
		"actions": []any{map[string]any{
			"type": "email", "to": "ada@example.com", "subject": map[string]any{"record": true},
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("schedule record %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "clock", "label": "Clock", "every": "1m",
		"actions": []any{map[string]any{"type": "email", "to": "ada@example.com", "subject": "Hi", "body": ""}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodDelete, "/api/automations/clock", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "copied_body", "label": "Copied body", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{
			"type":    "email",
			"to":      map[string]any{"field": "email"},
			"subject": map[string]any{"field": "name"},
			"body":    map[string]any{"field": "note"},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodDelete, "/api/automations/copied_body", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "reader", "label": "Reader",
		"grants": []any{map[string]any{"object": "lead", "can_read": true, "can_write": false}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var key struct {
		Token string `json:"token"`
	}
	mustJSON(t, rec, &key)
	rec = apiDo(t, http.MethodGet, "/api/emails", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	rec = apiDo(t, http.MethodGet, "/api/emails", key.Token, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("service %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/emails", member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("member %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "letter", "label": "Letter", "event": "record.created", "object": "lead",
		"actions": []any{map[string]any{
			"type":    "email",
			"to":      map[string]any{"field": "email"},
			"subject": map[string]any{"field": "name"},
			"body":    "",
		}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodGet, "/api/automations/letter", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var stored automationView
	mustJSON(t, rec, &stored)
	if len(stored.Actions) != 1 || stored.Actions[0].Type != "email" || stored.Actions[0].Body != "" {
		t.Fatalf("stored %+v", stored.Actions)
	}
	to, _ := stored.Actions[0].To.(map[string]any)
	subject, _ := stored.Actions[0].Subject.(map[string]any)
	if to["field"] != "email" || subject["field"] != "name" {
		t.Fatalf("shapes to %#v subject %#v", stored.Actions[0].To, stored.Actions[0].Subject)
	}
	if len(listEmails(t, token)) != 0 || len(listEmails(t, other)) != 0 {
		t.Fatal("email existed before the job")
	}

	lead := postRecord(t, token, "lead", map[string]any{"name": "Hello", "email": "ada@example.com", "note": ""})
	if len(listEmails(t, token)) != 0 {
		t.Fatal("the record request stored an email")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	mails := listEmails(t, token)
	if len(mails) != 1 || mails[0].To != "ada@example.com" || mails[0].Subject != "Hello" || mails[0].Body != "" || mails[0].CreatedAt.IsZero() {
		t.Fatalf("stored %+v", mails)
	}
	rec = apiDo(t, http.MethodGet, "/api/emails", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "sent") || strings.Contains(rec.Body.String(), "smtp") {
		t.Fatalf("response claims delivery %s", rec.Body.String())
	}
	var raw struct {
		Emails []map[string]any `json:"emails"`
	}
	mustJSON(t, rec, &raw)
	if len(raw.Emails) != 1 {
		t.Fatalf("list %d", len(raw.Emails))
	}
	item := raw.Emails[0]
	if len(item) != 4 || item["to"] != "ada@example.com" || item["subject"] != "Hello" || item["body"] != "" || item["created_at"] == "" {
		t.Fatalf("shape %#v", item)
	}
	rec = apiDo(t, http.MethodGet, "/api/emails", member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("member after store %s", rec.Body.String())
	}
	if len(listEmails(t, other)) != 0 {
		t.Fatal("another organization saw the email")
	}
	createdEvent := createdEventID(t, token, lead.ID)
	if _, err := uuid.Parse(createdEvent); err != nil {
		t.Fatal(err)
	}
	var gotOrg, gotTo, gotSubject, gotBody, gotEvent string
	if err := testPool.QueryRow(context.Background(), `
		select organization_id::text, to_address, subject, body, event_id::text
		from outbound_emails
		where event_id = $1::uuid
	`, createdEvent).Scan(&gotOrg, &gotTo, &gotSubject, &gotBody, &gotEvent); err != nil {
		t.Fatal(err)
	}
	if gotOrg != orgID || gotTo != "ada@example.com" || gotSubject != "Hello" || gotBody != "" || gotEvent != createdEvent {
		t.Fatalf("row org %s to %s subject %s body %s event %s", gotOrg, gotTo, gotSubject, gotBody, gotEvent)
	}
	ran := listExecutions(t, token, "letter")
	if len(ran) != 1 || ran[0].Status != "done" || ran[0].Path != "actions" || len(ran[0].Created) != 0 {
		t.Fatalf("execution %+v", ran)
	}
	done := listJobs(t, token, "event="+createdEvent+"&status=done")
	if len(done) != 1 || done[0].Kind != "automation" {
		t.Fatalf("job %+v", done)
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	var again int
	if err := testPool.QueryRow(context.Background(), `
		select count(*) from outbound_emails where event_id = $1::uuid
	`, createdEvent).Scan(&again); err != nil {
		t.Fatal(err)
	}
	if again != 1 || len(listEmails(t, token)) != 1 || len(listExecutions(t, token, "letter")) != 1 {
		t.Fatalf("second pass rows %d list %d", again, len(listEmails(t, token)))
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/letter", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listEmails(t, token)) != 1 {
		t.Fatal("deleting the workflow removed the email")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer bad.Close()
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "miss", "label": "Miss", "event": "record.created", "object": "lead",
		"actions": []any{
			map[string]any{"type": "email", "to": "ada@example.com", "subject": "Nope", "body": ""},
			map[string]any{"type": "http", "url": bad.URL},
		},
	})
	requireStatus(t, rec, http.StatusCreated)
	failedLead := postRecord(t, token, "lead", map[string]any{"name": "Boom", "email": "ada@example.com", "note": ""})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	failedEvent := createdEventID(t, token, failedLead.ID)
	if _, err := uuid.Parse(failedEvent); err != nil {
		t.Fatal(err)
	}
	var failedN int
	if err := testPool.QueryRow(context.Background(), `
		select count(*) from outbound_emails where event_id = $1::uuid
	`, failedEvent).Scan(&failedN); err != nil {
		t.Fatal(err)
	}
	pending := listJobs(t, token, "event="+failedEvent+"&status=pending")
	if failedN != 0 || len(listEmails(t, token)) != 1 || len(pending) != 1 || pending[0].Kind != "automation" || pending[0].Attempts != 1 || pending[0].LastError == nil || *pending[0].LastError != "http status 500" {
		t.Fatalf("failed rows %d jobs %+v", failedN, pending)
	}
	if len(listExecutions(t, token, "miss")) != 0 {
		t.Fatal("a failed action recorded an execution")
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/miss", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listJobs(t, token, "event="+failedEvent)) != 0 {
		t.Fatal("deleted workflow left its job")
	}

	var pendingN int
	if err := testPool.QueryRow(context.Background(), `
		select count(*) from jobs
		where organization_id = $1::uuid and status = 'pending'
	`, orgID).Scan(&pendingN); err != nil {
		t.Fatal(err)
	}
	if pendingN != 0 {
		t.Fatalf("left %d pending jobs", pendingN)
	}
}

func listEmails(t *testing.T, token string) []emailView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/emails", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Emails []emailView `json:"emails"`
	}
	mustJSON(t, rec, &body)
	if body.Emails == nil {
		t.Fatal("null emails")
	}
	return body.Emails
}
