package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestNotificationReachesTheRecipient(t *testing.T) {
	token := newOwner(t, "Notify", "notify-owner@example.com")
	orgID := orgOf(t, token)
	const memberEmail = "notify-member@example.com"
	const hiddenEmail = "notify-missing@example.com"
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `update users set email = $1 where email = $2`, memberEmail, hiddenEmail)
		_, _ = testPool.Exec(ctx, `delete from automation_definitions where organization_id = $1::uuid`, orgID)
		_, _ = testPool.Exec(ctx, `delete from jobs where organization_id = $1::uuid and status <> 'done'`, orgID)
	})
	postObject(t, token, companyObject())
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", memberEmail)
	if err != nil {
		t.Fatal(err)
	}
	memberID := userOf(t, member)
	other := newOwner(t, "Notify Other", "notify-other@example.com")

	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "missing_user", "label": "Missing", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "notification", "user": "nobody@example.com", "title": "Hi", "body": ""}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "not found" {
		t.Fatalf("unknown email %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "foreign_user", "label": "Foreign", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "notification", "user": "notify-other@example.com", "title": "Hi", "body": ""}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "not found" {
		t.Fatalf("other org %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/missing_user", token, nil)
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "blank_user", "label": "Blank", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "notification", "user": " ", "title": "Hi", "body": ""}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "user is required" {
		t.Fatalf("user %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "blank_title", "label": "Blank title", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "notification", "user": memberEmail, "title": "", "body": "x"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "title is required" {
		t.Fatalf("title %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "no_title", "label": "No title", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "notification", "user": memberEmail, "body": "x"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "title is required" {
		t.Fatalf("missing title %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "bad_field", "label": "Bad field", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{
			"type": "notification", "user": memberEmail,
			"title": map[string]any{"field": "missing"}, "body": "",
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown field missing" {
		t.Fatalf("field %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "extra", "label": "Extra", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "notification", "user": memberEmail, "title": "Hi", "object": "company"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("extra %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "clock_field", "label": "Clock field", "every": "1m",
		"actions": []any{map[string]any{
			"type": "notification", "user": memberEmail,
			"title": map[string]any{"field": "name"}, "body": "",
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("schedule field %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "clock_record", "label": "Clock record", "every": "1m",
		"actions": []any{map[string]any{
			"type": "notification", "user": memberEmail,
			"title": map[string]any{"record": true}, "body": "",
		}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("schedule record %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "clock", "label": "Clock", "every": "1m",
		"actions": []any{map[string]any{"type": "notification", "user": "Notify-Member@Example.com", "title": "Tick", "body": ""}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodDelete, "/api/automations/clock", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "reader", "label": "Reader",
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": false}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var key struct {
		Token string `json:"token"`
	}
	mustJSON(t, rec, &key)
	rec = apiDo(t, http.MethodGet, "/api/notifications", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	rec = apiDo(t, http.MethodGet, "/api/notifications", key.Token, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("service %s", rec.Body.String())
	}
	if len(listNotifications(t, token)) != 0 || len(listNotifications(t, member)) != 0 || len(listNotifications(t, other)) != 0 {
		t.Fatal("notifications existed before the job")
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "ping", "label": "Ping", "event": "record.created", "object": "company",
		"actions": []any{
			map[string]any{
				"type": "notification", "user": memberEmail,
				"title": map[string]any{"field": "name"}, "body": map[string]any{"record": true},
			},
			map[string]any{"type": "notification", "user": memberEmail, "title": "Second", "body": ""},
		},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodGet, "/api/automations/ping", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var stored automationView
	mustJSON(t, rec, &stored)
	if len(stored.Actions) != 2 || stored.Actions[0].User != memberEmail || stored.Actions[1].Title != "Second" || stored.Actions[1].Body != "" {
		t.Fatalf("stored %+v", stored.Actions)
	}
	title, _ := stored.Actions[0].Title.(map[string]any)
	body, _ := stored.Actions[0].Body.(map[string]any)
	if title["field"] != "name" || body["record"] != true {
		t.Fatalf("shapes title %#v body %#v", stored.Actions[0].Title, stored.Actions[0].Body)
	}

	company := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 3, "industry": "SaaS"})
	if len(listNotifications(t, member)) != 0 {
		t.Fatal("the record request wrote a notification")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	notes := listNotifications(t, member)
	if len(notes) != 2 || notes[0].Title != "Second" || notes[0].Body != "" || !notes[0].CreatedAt.After(notes[1].CreatedAt) {
		t.Fatalf("newest %+v", notes)
	}
	if notes[1].Title != "Acme" || notes[1].Body != company.ID {
		t.Fatalf("copied %+v", notes[1])
	}
	rec = apiDo(t, http.MethodGet, "/api/notifications", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var raw struct {
		Notifications []map[string]any `json:"notifications"`
	}
	mustJSON(t, rec, &raw)
	for _, item := range raw.Notifications {
		if len(item) != 4 || item["id"] == "" || item["title"] == "" || item["created_at"] == "" {
			t.Fatalf("shape %#v", item)
		}
		if _, ok := item["body"].(string); !ok {
			t.Fatalf("body %#v", item["body"])
		}
	}
	if len(listNotifications(t, token)) != 0 || len(listNotifications(t, other)) != 0 {
		t.Fatal("another user saw the notification")
	}
	for _, id := range []string{notes[1].ID, memberID, orgID} {
		if _, err := uuid.Parse(id); err != nil {
			t.Fatal(err)
		}
	}
	var gotOrg, gotUser, gotEvent, gotBody string
	if err := testPool.QueryRow(context.Background(), `
		select organization_id::text, user_id::text, event_id::text, body
		from notifications
		where id = $1::uuid
	`, notes[1].ID).Scan(&gotOrg, &gotUser, &gotEvent, &gotBody); err != nil {
		t.Fatal(err)
	}
	createdEvent := createdEventID(t, token, company.ID)
	if gotOrg != orgID || gotUser != memberID || gotEvent != createdEvent || gotBody != company.ID {
		t.Fatalf("row org %s user %s event %s body %s", gotOrg, gotUser, gotEvent, gotBody)
	}
	ran := listExecutions(t, token, "ping")
	if len(ran) != 1 || ran[0].Status != "done" || ran[0].Path != "actions" || len(ran[0].Created) != 0 {
		t.Fatalf("execution %+v", ran)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/ping", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listNotifications(t, member)) != 2 {
		t.Fatal("deleting the workflow removed the notification")
	}

	var mu sync.Mutex
	var hits int
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		hits++
		mu.Unlock()
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer bad.Close()
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "miss", "label": "Miss", "event": "record.created", "object": "company",
		"actions": []any{
			map[string]any{"type": "notification", "user": memberEmail, "title": "Nope", "body": ""},
			map[string]any{"type": "http", "url": bad.URL},
		},
	})
	requireStatus(t, rec, http.StatusCreated)
	failedRecord := postRecord(t, token, "company", map[string]any{"name": "Boom", "employees": 1, "industry": "Finance"})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	failedEvent := createdEventID(t, token, failedRecord.ID)
	if _, err := uuid.Parse(failedEvent); err != nil {
		t.Fatal(err)
	}
	var failedN int
	if err := testPool.QueryRow(context.Background(), `
		select count(*) from notifications where event_id = $1::uuid
	`, failedEvent).Scan(&failedN); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	seen := hits
	mu.Unlock()
	if failedN != 0 || seen != 1 || len(listNotifications(t, member)) != 2 {
		t.Fatalf("failed attempt rows %d hits %d list %d", failedN, seen, len(listNotifications(t, member)))
	}
	if len(listExecutions(t, token, "miss")) != 0 {
		t.Fatal("a failed action recorded an execution")
	}
	pending := listJobs(t, token, "event="+failedEvent+"&status=pending")
	if len(pending) != 1 || pending[0].Kind != "automation" || pending[0].Attempts != 1 || pending[0].LastError == nil || *pending[0].LastError != "http status 500" {
		t.Fatalf("pending %+v", pending)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/miss", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listJobs(t, token, "event="+failedEvent)) != 0 {
		t.Fatal("deleted workflow left its job")
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "gone", "label": "Gone", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "notification", "user": memberEmail, "title": "Later", "body": ""}},
	})
	requireStatus(t, rec, http.StatusCreated)
	tag, err := testPool.Exec(context.Background(), `update users set email = $1 where email = $2`, hiddenEmail, memberEmail)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("email rows %d", tag.RowsAffected())
	}
	lost := postRecord(t, token, "company", map[string]any{"name": "Lost", "employees": 2, "industry": "SaaS"})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	lostEvent := createdEventID(t, token, lost.ID)
	if _, err := uuid.Parse(lostEvent); err != nil {
		t.Fatal(err)
	}
	var lostN int
	if err := testPool.QueryRow(context.Background(), `
		select count(*) from notifications where event_id = $1::uuid
	`, lostEvent).Scan(&lostN); err != nil {
		t.Fatal(err)
	}
	lostJobs := listJobs(t, token, "event="+lostEvent+"&status=pending")
	if lostN != 0 || len(listNotifications(t, member)) != 2 || len(lostJobs) != 1 || lostJobs[0].LastError == nil || *lostJobs[0].LastError != "not found" {
		t.Fatalf("missing user rows %d jobs %+v", lostN, lostJobs)
	}
	if _, err := testPool.Exec(context.Background(), `update users set email = $1 where email = $2`, memberEmail, hiddenEmail); err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/gone", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

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

func listNotifications(t *testing.T, token string) []notificationView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/notifications", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Notifications []notificationView `json:"notifications"`
	}
	mustJSON(t, rec, &body)
	if body.Notifications == nil {
		t.Fatal("null notifications")
	}
	return body.Notifications
}
