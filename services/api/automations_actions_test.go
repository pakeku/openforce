package main

import (
	"context"
	"net/http"
	"testing"
)

func TestAutomationCanUpdateAndDelete(t *testing.T) {
	token := newOwner(t, "Actions", "actions-owner@example.com")
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "note",
		"label": "Note",
		"fields": []map[string]any{
			{"name": "body", "type": "text", "required": true},
		},
	})

	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "call", "label": "Call", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "email", "object": "company"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("type %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "blank", "label": "Blank", "event": "record.updated", "object": "company",
		"actions": []any{map[string]any{"type": "update", "data": map[string]any{}}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "data is required" {
		t.Fatalf("data %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "shout", "label": "Shout", "event": "record.updated", "object": "note",
		"actions": []any{map[string]any{"type": "update", "data": map[string]any{"body": "LOUD"}}},
	})
	requireStatus(t, rec, http.StatusCreated)
	note := postRecord(t, token, "note", map[string]any{"body": "quiet"})
	rec = apiDo(t, http.MethodPatch, "/api/objects/note/"+note.ID, token, map[string]any{
		"version": 1,
		"data":    map[string]any{"body": "soft"},
	})
	requireStatus(t, rec, http.StatusOK)
	if getRecord(t, token, "note", note.ID).Data["body"] != "soft" {
		t.Fatal("the patch ran the workflow")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	shouted := getRecord(t, token, "note", note.ID)
	if shouted.Data["body"] != "LOUD" || shouted.Version != 3 {
		t.Fatalf("note %+v", shouted)
	}
	var caused string
	for _, ev := range eventsFor(listEvents(t, token, "record="+note.ID), note.ID) {
		if ev.Name == "record.updated" && ev.Source == "Automation" {
			caused = ev.ID
		}
	}
	if caused == "" {
		t.Fatal("missing automation update")
	}
	if len(listJobs(t, token, "event="+caused)) != 0 {
		t.Fatal("workflow retriggered itself")
	}
	if n, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil || n != 0 {
		t.Fatalf("second delivery %d %v", n, err)
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "stamp", "label": "Stamp", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "update", "data": map[string]any{"industry": "Finance"}}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "close", "label": "Close", "event": "record.updated", "object": "company",
		"conditions": []any{map[string]any{"field": "industry", "op": "eq", "value": "Finance"}},
		"actions":    []any{map[string]any{"type": "delete"}},
	})
	requireStatus(t, rec, http.StatusCreated)
	company := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 3, "industry": "SaaS"})
	if getRecord(t, token, "company", company.ID).Data["industry"] != "SaaS" {
		t.Fatal("create ran the workflow")
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+company.ID, token, nil)
	requireStatus(t, rec, http.StatusNotFound)
	var sawFinance, sawDelete bool
	for _, entry := range listAudit(t, token, company.ID) {
		if entry.Field != nil && *entry.Field == "industry" && entry.After == "Finance" && entry.Source == "Automation" {
			sawFinance = true
		}
		if entry.Name == "record.deleted" && entry.Source == "Automation" {
			sawDelete = true
		}
	}
	if !sawFinance || !sawDelete {
		t.Fatal("audit is missing the workflow update or delete")
	}
	for _, name := range []string{"stamp", "close"} {
		executions := listExecutions(t, token, name)
		if len(executions) != 1 || executions[0].Status != "done" {
			t.Fatalf("%s executions %+v", name, executions)
		}
	}
	if n, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil || n != 0 {
		t.Fatalf("follow-up delivery %d %v", n, err)
	}
}
