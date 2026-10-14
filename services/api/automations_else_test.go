package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestAutomationTakesTheOtherPath(t *testing.T) {
	var mu sync.Mutex
	var matchHits, missHits int
	matchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		matchHits++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer matchSrv.Close()
	missSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		missHits++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer missSrv.Close()

	token := newOwner(t, "Else", "else-owner@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `
			delete from automation_definitions
			where organization_id = $1::uuid
		`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "task",
		"label": "Task",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "company", "type": "text"},
		},
	})
	postObject(t, token, map[string]any{
		"name":  "lead",
		"label": "Lead",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "source", "type": "text"},
		},
	})
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "else-member@example.com")
	if err != nil {
		t.Fatal(err)
	}

	rec := apiDo(t, http.MethodPost, "/api/automations", member, map[string]any{
		"name": "branch", "label": "Branch", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "update", "data": map[string]any{"industry": "Finance"}}},
		"else":    []any{map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Nope"}}},
	})
	requireStatus(t, rec, http.StatusForbidden)
	if errorOf(t, rec) != "forbidden" || strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("member %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "ruled", "label": "Ruled", "every": "1m",
		"actions": []any{map[string]any{"type": "http", "url": "https://example.com/hook"}},
		"else":    []any{map[string]any{"type": "http", "url": "https://example.com/other"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid schedule" {
		t.Fatalf("schedule else %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "ruled", "label": "Ruled", "every": "1m",
		"actions": []any{map[string]any{"type": "http", "url": "https://example.com/hook"}},
		"else":    []any{map[string]any{"type": "update", "data": map[string]any{"industry": "Finance"}}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid schedule" {
		t.Fatalf("schedule update else %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "plain", "label": "Plain", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "delete"}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var plain automationView
	mustJSON(t, rec, &plain)
	if plain.Else == nil || len(plain.Else) != 0 {
		t.Fatalf("omitted else %#v", plain.Else)
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/plain", token, nil)
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &plain)
	if plain.Else == nil || len(plain.Else) != 0 {
		t.Fatalf("stored else %#v", plain.Else)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/plain", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	var objectID string
	if err := testPool.QueryRow(context.Background(), `
		select id::text from object_definitions
		where organization_id = $1::uuid and name = 'company'
	`, orgID).Scan(&objectID); err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(objectID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(context.Background(), `
		insert into automation_definitions
			(id, organization_id, name, label, event_name, object_id, conditions, actions)
		values ($1::uuid, $2::uuid, 'legacy', 'Legacy', 'record.created', $3::uuid, $4::jsonb, $5::jsonb)
	`, uuid.NewString(), orgID, objectID, "[]", `[{"type":"delete"}]`); err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/legacy", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var legacy automationView
	mustJSON(t, rec, &legacy)
	if legacy.Else == nil || len(legacy.Else) != 0 {
		t.Fatalf("legacy else %#v", legacy.Else)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/legacy", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "rewrite", "label": "Rewrite", "event": "record.updated", "object": "company",
		"actions": []any{map[string]any{"type": "http", "url": "https://example.com/hook"}},
		"else": []any{
			map[string]any{"type": "update", "data": map[string]any{"industry": "Finance"}},
			map[string]any{"type": "delete"},
		},
	})
	requireStatus(t, rec, http.StatusCreated)
	var rewrite automationView
	mustJSON(t, rec, &rewrite)
	if len(rewrite.Else) != 2 || rewrite.Else[0].Type != "update" || rewrite.Else[1].Type != "delete" {
		t.Fatalf("else actions %+v", rewrite.Else)
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "bad_else", "label": "Bad else", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "delete"}},
		"else":    []any{map[string]any{"type": "email"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "to is required" {
		t.Fatalf("else type %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/rewrite", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "branch", "label": "Branch", "event": "record.created", "object": "company",
		"conditions": []any{map[string]any{"field": "employees", "op": "gte", "value": 5}},
		"actions": []any{
			map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Matched", "company": map[string]any{"record": true}}},
			map[string]any{"type": "http", "url": matchSrv.URL},
		},
		"else": []any{
			map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Missed", "company": map[string]any{"record": true}}},
			map[string]any{"type": "http", "url": missSrv.URL},
		},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPatch, "/api/automations/branch", member, map[string]any{"version": 1, "else": []any{}})
	requireStatus(t, rec, http.StatusForbidden)
	if errorOf(t, rec) != "forbidden" {
		t.Fatalf("member patch %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPatch, "/api/automations/branch", token, map[string]any{"version": 1, "label": "Branched"})
	requireStatus(t, rec, http.StatusOK)
	var branched automationView
	mustJSON(t, rec, &branched)
	if branched.Label != "Branched" || len(branched.Else) != 2 || branched.Else[0].Type != "create" {
		t.Fatalf("patch kept else %+v", branched)
	}

	big := postRecord(t, token, "company", map[string]any{"name": "Big", "employees": 5, "industry": "SaaS"})
	if len(listRecords(t, token, "task", "")) != 0 {
		t.Fatal("the record request ran the workflow")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	tasks := listRecords(t, token, "task", "")
	if len(tasks) != 1 || tasks[0].Data["name"] != "Matched" || tasks[0].Data["company"] != big.ID {
		t.Fatalf("match tasks %#v", tasks)
	}
	mu.Lock()
	if matchHits != 1 || missHits != 0 {
		t.Fatalf("match calls match %d miss %d", matchHits, missHits)
	}
	mu.Unlock()
	ran := listExecutions(t, token, "branch")
	if len(ran) != 1 || ran[0].Status != "done" || ran[0].Path != "actions" || len(ran[0].Created) != 1 || ran[0].Created[0] != tasks[0].ID {
		t.Fatalf("match execution %+v", ran)
	}

	tiny := postRecord(t, token, "company", map[string]any{"name": "Tiny", "employees": 1, "industry": "SaaS"})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	tasks = listRecords(t, token, "task", "")
	if len(tasks) != 2 || !containsTask(tasks, "Missed", tiny.ID) || !containsTask(tasks, "Matched", big.ID) {
		t.Fatalf("miss tasks %#v", tasks)
	}
	var missedID string
	for _, task := range tasks {
		if task.Data["name"] == "Missed" {
			missedID = task.ID
		}
	}
	mu.Lock()
	if matchHits != 1 || missHits != 1 {
		t.Fatalf("miss calls match %d miss %d", matchHits, missHits)
	}
	mu.Unlock()
	ran = listExecutions(t, token, "branch")
	if len(ran) != 2 {
		t.Fatalf("executions %+v", ran)
	}
	var matchedExec, missedExec executionView
	for _, execution := range ran {
		switch execution.Path {
		case "actions":
			matchedExec = execution
		case "else":
			missedExec = execution
		}
	}
	tinyEvent := eventsNamed(eventsFor(listEvents(t, token, "record="+tiny.ID), tiny.ID), "record.created")
	if len(tinyEvent) != 1 || missedExec.Status != "done" || missedExec.EventID != tinyEvent[0].ID || len(missedExec.Created) != 1 || missedExec.Created[0] != missedID {
		t.Fatalf("else execution %+v event %+v", missedExec, tinyEvent)
	}
	if matchedExec.Status != "done" || len(matchedExec.Created) != 1 || matchedExec.Created[0] == missedID {
		t.Fatalf("actions execution %+v", matchedExec)
	}
	if getRecord(t, token, "company", tiny.ID).Data["industry"] != "SaaS" {
		t.Fatal("the miss ran the match path")
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/branch", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "quiet", "label": "Quiet", "event": "record.created", "object": "company",
		"conditions": []any{map[string]any{"field": "employees", "op": "gte", "value": 1000}},
		"actions":    []any{map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Too big"}}},
		"else":       []any{map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Should not"}}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPatch, "/api/automations/quiet", token, map[string]any{
		"version": 1,
		"else":    []any{},
	})
	requireStatus(t, rec, http.StatusOK)
	var quiet automationView
	mustJSON(t, rec, &quiet)
	if quiet.Else == nil || len(quiet.Else) != 0 {
		t.Fatalf("cleared else %#v", quiet.Else)
	}
	before := len(listRecords(t, token, "task", ""))
	postRecord(t, token, "company", map[string]any{"name": "Small", "employees": 1, "industry": "SaaS"})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(listRecords(t, token, "task", "")) != before {
		t.Fatal("an empty else ran an action")
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/quiet/executions", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), `"path"`) {
		t.Fatalf("skipped path %s", rec.Body.String())
	}
	skipped := listExecutions(t, token, "quiet")
	if len(skipped) != 1 || skipped[0].Status != "skipped" || skipped[0].Path != "" || len(skipped[0].Created) != 0 {
		t.Fatalf("skipped %+v", skipped)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/quiet", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "echo", "label": "Echo", "event": "record.created", "object": "lead",
		"conditions": []any{map[string]any{"field": "name", "op": "eq", "value": "Start"}},
		"actions":    []any{map[string]any{"type": "update", "data": map[string]any{"source": "kept"}}},
		"else":       []any{map[string]any{"type": "create", "object": "lead", "data": map[string]any{"name": "Child"}}},
	})
	requireStatus(t, rec, http.StatusCreated)
	start := postRecord(t, token, "lead", map[string]any{"name": "Start"})
	drop := postRecord(t, token, "lead", map[string]any{"name": "Drop"})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if getRecord(t, token, "lead", start.ID).Data["source"] != "kept" {
		t.Fatal("match did not run actions")
	}
	if getRecord(t, token, "lead", drop.ID).Data["source"] != nil {
		t.Fatal("miss ran actions")
	}
	leads := listRecords(t, token, "lead", "")
	var child recordView
	children := 0
	for _, lead := range leads {
		if lead.Data["name"] == "Child" {
			child = lead
			children++
		}
	}
	if children != 1 {
		t.Fatalf("children %d in %#v", children, leads)
	}
	echoed := listExecutions(t, token, "echo")
	if len(echoed) != 2 {
		t.Fatalf("echo executions %+v", echoed)
	}
	for _, execution := range echoed {
		if execution.Path == "else" && (execution.Status != "done" || len(execution.Created) != 1 || execution.Created[0] != child.ID) {
			t.Fatalf("echo else %+v", execution)
		}
		if execution.Path == "actions" && (execution.Status != "done" || len(execution.Created) != 0) {
			t.Fatalf("echo actions %+v", execution)
		}
	}
	childEvent := eventsNamed(eventsFor(listEvents(t, token, "record="+child.ID), child.ID), "record.created")
	if len(childEvent) != 1 || childEvent[0].Source != "Automation" || len(listJobs(t, token, "event="+childEvent[0].ID)) != 0 {
		t.Fatalf("else retriggered %+v", childEvent)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/echo", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", token, map[string]any{
		"role": "member", "can_read": true, "can_write": true,
	})
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "guard", "label": "Guard", "event": "record.created", "object": "company",
		"conditions": []any{map[string]any{"field": "name", "op": "eq", "value": "Blocked"}},
		"actions":    []any{map[string]any{"type": "update", "data": map[string]any{"industry": "Finance"}}},
		"else":       []any{map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Nope"}}},
	})
	requireStatus(t, rec, http.StatusCreated)
	opened := postRecord(t, member, "company", map[string]any{"name": "Open", "employees": 1, "industry": "SaaS"})
	tasksBefore := len(listRecords(t, token, "task", ""))
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(listRecords(t, token, "task", "")) != tasksBefore {
		t.Fatal("else ignored the policy check")
	}
	openedEvent := eventsNamed(eventsFor(listEvents(t, token, "record="+opened.ID), opened.ID), "record.created")
	if len(openedEvent) != 1 {
		t.Fatalf("opened events %d", len(openedEvent))
	}
	failed := listJobs(t, token, "event="+openedEvent[0].ID)
	if len(failed) != 1 || failed[0].Status != "pending" || failed[0].LastError == nil || !strings.Contains(*failed[0].LastError, "forbidden") {
		t.Fatalf("else policy %+v", failed)
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/guard", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listJobs(t, token, "event="+openedEvent[0].ID)) != 0 {
		t.Fatal("deleted workflow left its job")
	}

	var due int
	if err := testPool.QueryRow(context.Background(), `
		select count(*) from jobs
		where organization_id = $1::uuid and status = 'pending' and run_at <= now()
	`, orgID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if due != 0 {
		t.Fatalf("left %d due jobs", due)
	}
}
