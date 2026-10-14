package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAutomationFollowsTheEvent(t *testing.T) {
	token := newOwner(t, "Automation", "automation-owner@example.com")
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "task",
		"label": "Task",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "company", "type": "text"},
		},
	})
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgOf(t, token), "Mia", "automation-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec := apiDo(t, http.MethodPost, "/api/automations", member, followUpAutomation(10))
	requireStatus(t, rec, http.StatusForbidden)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "early", "label": "Early", "event": "workflow.started", "object": "company",
		"actions": []any{map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "x"}}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid event" {
		t.Fatalf("event %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "empty", "label": "Empty", "event": "record.created", "object": "company",
		"actions": []any{},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "actions are required" {
		t.Fatalf("actions %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "bad_field", "label": "Bad", "event": "record.created", "object": "company",
		"conditions": []any{map[string]any{"field": "no_such", "op": "eq", "value": "x"}},
		"actions":    []any{map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "x"}}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown field no_such" {
		t.Fatalf("field %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, followUpAutomation(10))
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, followUpAutomation(10))
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "automation already exists" {
		t.Fatalf("duplicate %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "gone", "label": "Gone", "event": "record.deleted", "object": "company",
		"conditions": []any{map[string]any{"field": "name", "op": "eq", "value": "Tiny"}},
		"actions": []any{map[string]any{
			"type": "create", "object": "task",
			"data": map[string]any{"name": "Gone", "company": map[string]any{"record": true}},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)

	rec = apiDo(t, http.MethodPatch, "/api/automations/follow_up", token, map[string]any{
		"version":    2,
		"conditions": []any{map[string]any{"field": "employees", "op": "gte", "value": 5}},
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "stale version" {
		t.Fatalf("stale %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/follow_up", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var stored automationView
	mustJSON(t, rec, &stored)
	if stored.Version != 1 || stored.Event != "record.created" || stored.Object != "company" {
		t.Fatalf("stored %+v", stored)
	}
	rec = apiDo(t, http.MethodPatch, "/api/automations/follow_up", token, map[string]any{
		"version":    1,
		"conditions": []any{map[string]any{"field": "employees", "op": "gte", "value": 5}},
	})
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &stored)
	if stored.Version != 2 || len(stored.Conditions) != 1 || stored.Conditions[0].Op != "gte" {
		t.Fatalf("patched %+v", stored)
	}

	tiny := postRecord(t, token, "company", map[string]any{"name": "Tiny", "employees": 4, "industry": "SaaS"})
	big := postRecord(t, token, "company", map[string]any{"name": "Big", "employees": 5, "industry": "SaaS"})
	if len(listRecords(t, token, "task", "")) != 0 || len(listExecutions(t, token, "follow_up")) != 0 {
		t.Fatal("the record request ran the workflow")
	}
	created := eventsNamed(eventsFor(listEvents(t, token, "record="+big.ID), big.ID), "record.created")
	if len(created) != 1 {
		t.Fatalf("created events %d", len(created))
	}
	jobs := listJobs(t, token, "event="+created[0].ID)
	if len(jobs) != 1 || jobs[0].Kind != "automation" || jobs[0].Status != "pending" {
		t.Fatalf("job %+v", jobs)
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	tasks := listRecords(t, token, "task", "")
	if len(tasks) != 1 || tasks[0].Data["name"] != "Follow up" || tasks[0].Data["company"] != big.ID {
		t.Fatalf("tasks %#v", tasks)
	}
	executions := listExecutions(t, token, "follow_up")
	if len(executions) != 2 {
		t.Fatalf("executions %+v", executions)
	}
	var skipped, done executionView
	for _, execution := range executions {
		switch execution.Status {
		case "skipped":
			skipped = execution
		case "done":
			done = execution
		}
	}
	if skipped.EventID == "" || len(skipped.Created) != 0 || done.EventID != created[0].ID || len(done.Created) != 1 || done.Created[0] != tasks[0].ID {
		t.Fatalf("skipped %+v done %+v", skipped, done)
	}
	taskEvents := eventsNamed(eventsFor(listEvents(t, token, "record="+tasks[0].ID), tasks[0].ID), "record.created")
	if len(taskEvents) != 1 || taskEvents[0].Source != "Automation" {
		t.Fatalf("task event %+v", taskEvents)
	}
	jobs = listJobs(t, token, "event="+created[0].ID+"&status=done")
	if len(jobs) != 1 || jobs[0].Attempts != 1 {
		t.Fatalf("done job %+v", jobs)
	}

	rec = apiDo(t, http.MethodDelete, "/api/objects/company/"+tiny.ID, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listRecords(t, token, "task", "")) != 1 {
		t.Fatal("delete ran the workflow")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	tasks = listRecords(t, token, "task", "")
	if len(tasks) != 2 || !containsTask(tasks, "Gone", tiny.ID) {
		t.Fatalf("after delete %#v", tasks)
	}

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", token, map[string]any{
		"role": "member", "can_read": true, "can_write": true,
	})
	requireStatus(t, rec, http.StatusOK)
	memberCompany := postRecord(t, member, "company", map[string]any{"name": "Member Co", "employees": 9, "industry": "Finance"})
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(listRecords(t, token, "task", "")) != 2 {
		t.Fatal("member workflow wrote a task")
	}
	memberEvent := eventsNamed(eventsFor(listEvents(t, token, "record="+memberCompany.ID), memberCompany.ID), "record.created")
	if len(memberEvent) != 1 {
		t.Fatalf("member events %d", len(memberEvent))
	}
	failed := listJobs(t, token, "event="+memberEvent[0].ID)
	if len(failed) != 1 || failed[0].Status != "pending" || failed[0].Attempts != 1 || failed[0].LastError == nil || !strings.Contains(*failed[0].LastError, "forbidden") {
		t.Fatalf("failed job %+v", failed)
	}

	rec = apiDo(t, http.MethodGet, "/api/automations/follow_up", member, nil)
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodDelete, "/api/automations/follow_up", member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	other := newOwner(t, "Automation Other", "automation-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/automations/follow_up", other, nil)
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodGet, "/api/automations/follow_up/executions", other, nil)
	requireStatus(t, rec, http.StatusNotFound)

	rec = apiDo(t, http.MethodDelete, "/api/automations/follow_up", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if len(listJobs(t, token, "event="+memberEvent[0].ID)) != 0 {
		t.Fatal("deleted workflow left its job")
	}
	rec = apiDo(t, http.MethodGet, "/api/automations", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var listed struct {
		Automations []automationView `json:"automations"`
	}
	mustJSON(t, rec, &listed)
	if len(listed.Automations) != 1 || listed.Automations[0].Name != "gone" {
		t.Fatalf("list %+v", listed.Automations)
	}
}

func followUpAutomation(employees int) map[string]any {
	return map[string]any{
		"name": "follow_up", "label": "Follow up", "event": "record.created", "object": "company",
		"conditions": []any{map[string]any{"field": "employees", "op": "gte", "value": employees}},
		"actions": []any{map[string]any{
			"type": "create", "object": "task",
			"data": map[string]any{"name": "Follow up", "company": map[string]any{"record": true}},
		}},
	}
}

func listExecutions(t *testing.T, token, name string) []executionView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/automations/"+name+"/executions", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Executions []executionView `json:"executions"`
	}
	mustJSON(t, rec, &body)
	if body.Executions == nil {
		t.Fatal("null executions")
	}
	return body.Executions
}

func containsTask(tasks []recordView, name, company string) bool {
	for _, task := range tasks {
		if task.Data["name"] == name && task.Data["company"] == company {
			return true
		}
	}
	return false
}
