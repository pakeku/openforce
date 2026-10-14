package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestFunctionRunsOnTheWorker(t *testing.T) {
	token := newOwner(t, "Functions", "functions-owner@example.com")
	orgID := orgOf(t, token)
	names := []string{"stamp", "quiet", "explode", "deny", "tick"}
	t.Cleanup(func() {
		clearFunctionWork(t, orgID)
		for _, name := range names {
			clearFunction(name)
		}
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
		"name":  "note",
		"label": "Note",
		"fields": []map[string]any{
			{"name": "body", "type": "text", "required": true},
		},
	})
	postObject(t, token, map[string]any{
		"name":  "secret",
		"label": "Secret",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})
	for _, object := range []string{"company", "task"} {
		rec := apiDo(t, http.MethodPut, "/api/meta/objects/"+object+"/permissions", token, map[string]any{
			"role": "member", "can_read": true, "can_write": true,
		})
		requireStatus(t, rec, http.StatusOK)
	}
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "functions-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	memberID := userOf(t, member)
	ownerID := userOf(t, token)

	rec := apiDo(t, http.MethodPost, "/api/functions", member, map[string]any{"name": "stamp", "label": "Stamp"})
	requireStatus(t, rec, http.StatusForbidden)
	if errorOf(t, rec) != "forbidden" || strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("member register %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/functions", token, map[string]any{"name": "Stamp", "label": "Stamp"})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid name" {
		t.Fatalf("name %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/functions", token, map[string]any{"name": "stamp", "label": ""})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "label is required" {
		t.Fatalf("label %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/functions", token, map[string]any{"name": "stamp", "label": "Stamp"})
	requireStatus(t, rec, http.StatusCreated)
	var created map[string]any
	mustJSON(t, rec, &created)
	id, _ := created["id"].(string)
	if len(created) != 3 || created["name"] != "stamp" || created["label"] != "Stamp" || id == "" {
		t.Fatalf("created %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/functions", token, map[string]any{"name": "stamp", "label": "Again"})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "function already exists" {
		t.Fatalf("duplicate %s", rec.Body.String())
	}
	for _, function := range []struct{ name, label string }{
		{"quiet", "Quiet"},
		{"explode", "Explode"},
		{"deny", "Deny"},
		{"tick", "Tick"},
		{"temp", "Temp"},
	} {
		rec = apiDo(t, http.MethodPost, "/api/functions", token, map[string]any{"name": function.name, "label": function.label})
		requireStatus(t, rec, http.StatusCreated)
	}

	rec = apiDo(t, http.MethodGet, "/api/functions", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var listed struct {
		Functions []functionView `json:"functions"`
	}
	mustJSON(t, rec, &listed)
	if len(listed.Functions) != 6 || listed.Functions[0].Name != "deny" || listed.Functions[0].Label != "Deny" {
		t.Fatalf("list %+v", listed.Functions)
	}
	other := newOwner(t, "Functions Other", "functions-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/functions", other, nil)
	requireStatus(t, rec, http.StatusOK)
	var foreign struct {
		Functions []functionView `json:"functions"`
	}
	mustJSON(t, rec, &foreign)
	if len(foreign.Functions) != 0 {
		t.Fatalf("other org %+v", foreign.Functions)
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", other, map[string]any{
		"name": "foreign", "label": "Foreign", "every": "1m",
		"actions": []any{map[string]any{"type": "function", "name": "stamp"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "not found" {
		t.Fatalf("other workflow %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodDelete, "/api/functions/temp", member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("member delete %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodDelete, "/api/functions/temp", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodDelete, "/api/functions/temp", token, nil)
	requireStatus(t, rec, http.StatusNotFound)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, functionWorkflow("missing", "Missing", "record.created", "company", "missing", "Acme"))
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "not found" {
		t.Fatalf("missing %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, functionWorkflow("bad", "Bad", "record.created", "company", "Bad-Name", ""))
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid name" {
		t.Fatalf("bad name %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "shaped", "label": "Shaped", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "function", "name": "stamp", "object": "task"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("shape %s", rec.Body.String())
	}

	var scheduleSawRecord bool
	registerFunction("stamp", func(ctx context.Context, event functionEvent) error {
		name, _ := event.Data["name"].(string)
		_, err := event.Create("task", map[string]any{"name": name, "company": event.RecordID})
		return err
	})
	registerFunction("explode", func(ctx context.Context, event functionEvent) error {
		return fmt.Errorf("handler broke")
	})
	registerFunction("quiet", nil)
	registerFunction("deny", func(ctx context.Context, event functionEvent) error {
		_, err := event.Create("secret", map[string]any{"name": "nope"})
		return err
	})
	registerFunction("tick", func(ctx context.Context, event functionEvent) error {
		if event.Name != "schedule.tick" || event.RecordID != "" || event.Object != "" {
			scheduleSawRecord = true
			return fmt.Errorf("schedule had a record")
		}
		id, err := event.Create("note", map[string]any{"body": "tick"})
		if err != nil {
			return err
		}
		return event.Update("note", id, map[string]any{"body": "tock"})
	})

	rec = apiDo(t, http.MethodPost, "/api/automations", token, functionWorkflow("stamp", "Stamp", "record.created", "company", "stamp", "Acme"))
	requireStatus(t, rec, http.StatusCreated)
	var stored automationView
	mustJSON(t, rec, &stored)
	if len(stored.Actions) != 1 || stored.Actions[0].Type != "function" || stored.Actions[0].Name != "stamp" {
		t.Fatalf("stored %+v", stored.Actions)
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, functionWorkflow("explode", "Explode", "record.created", "company", "explode", "Boom"))
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, functionWorkflow("quiet", "Quiet", "record.created", "company", "quiet", "Ghost"))
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, functionWorkflow("deny", "Deny", "record.created", "company", "deny", "Denied"))
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, functionWorkflow("tick", "Tick", "", "", "tick", ""))
	requireStatus(t, rec, http.StatusCreated)

	acme := postRecord(t, member, "company", map[string]any{"name": "Acme"})
	boom := postRecord(t, member, "company", map[string]any{"name": "Boom"})
	ghost := postRecord(t, member, "company", map[string]any{"name": "Ghost"})
	denied := postRecord(t, member, "company", map[string]any{"name": "Denied"})
	if len(listRecords(t, token, "task", "")) != 0 {
		t.Fatal("the record request ran the function")
	}
	if _, err := testPool.Exec(context.Background(), `
		update automation_definitions
		set next_run_at = now() - interval '1 second'
		where organization_id = $1::uuid and name = 'tick'
	`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&server{pool: testPool}).enqueueDueSchedules(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}

	if scheduleSawRecord {
		t.Fatal("schedule function saw a triggering record")
	}
	tasks := listRecords(t, token, "task", "")
	if len(tasks) != 1 || tasks[0].Data["name"] != "Acme" || tasks[0].Data["company"] != acme.ID {
		t.Fatalf("tasks %#v", tasks)
	}
	taskEvent := eventsNamed(eventsFor(listEvents(t, token, "record="+tasks[0].ID), tasks[0].ID), "record.created")
	if len(taskEvent) != 1 || taskEvent[0].Source != "Automation" || taskEvent[0].ActorID != memberID {
		t.Fatalf("task event %+v", taskEvent)
	}
	acmeEvent := createdEventID(t, token, acme.ID)
	ran, ok := executionFor(listExecutions(t, token, "stamp"), acmeEvent)
	if !ok || ran.Status != "done" || ran.Path != "actions" || len(ran.Created) != 1 || ran.Created[0] != tasks[0].ID {
		t.Fatalf("stamp execution %+v", ran)
	}

	boomEvent := createdEventID(t, token, boom.ID)
	if _, ok := executionFor(listExecutions(t, token, "explode"), boomEvent); ok {
		t.Fatal("handler error wrote an execution")
	}
	boomJob := pendingAutomation(t, listJobs(t, token, "event="+boomEvent))
	if boomJob.Attempts != 1 || boomJob.LastError == nil || *boomJob.LastError != "handler broke" {
		t.Fatalf("explode job %+v", boomJob)
	}

	ghostEvent := createdEventID(t, token, ghost.ID)
	if _, ok := executionFor(listExecutions(t, token, "quiet"), ghostEvent); ok {
		t.Fatal("unknown function wrote an execution")
	}
	ghostJob := pendingAutomation(t, listJobs(t, token, "event="+ghostEvent))
	if ghostJob.Attempts != 1 || ghostJob.LastError == nil || *ghostJob.LastError != "unknown function" {
		t.Fatalf("quiet job %+v", ghostJob)
	}

	deniedEvent := createdEventID(t, token, denied.ID)
	if _, ok := executionFor(listExecutions(t, token, "deny"), deniedEvent); ok {
		t.Fatal("forbidden function wrote an execution")
	}
	deniedJob := pendingAutomation(t, listJobs(t, token, "event="+deniedEvent))
	if deniedJob.Attempts != 1 || deniedJob.LastError == nil || *deniedJob.LastError != "forbidden" {
		t.Fatalf("deny job %+v", deniedJob)
	}
	if len(listRecords(t, token, "secret", "")) != 0 {
		t.Fatal("policy did not stop the function")
	}

	notes := listRecords(t, token, "note", "")
	if len(notes) != 1 || notes[0].Data["body"] != "tock" || notes[0].Version != 2 {
		t.Fatalf("notes %+v", notes)
	}
	noteEvents := eventsFor(listEvents(t, token, "record="+notes[0].ID), notes[0].ID)
	noteCreated := eventsNamed(noteEvents, "record.created")
	noteUpdated := eventsNamed(noteEvents, "record.updated")
	if len(noteCreated) != 1 || noteCreated[0].Source != "Automation" || noteCreated[0].ActorID != ownerID {
		t.Fatalf("note created %+v", noteCreated)
	}
	if len(noteUpdated) != 1 || noteUpdated[0].Source != "Automation" || noteUpdated[0].ActorID != ownerID {
		t.Fatalf("note updated %+v", noteUpdated)
	}
	tickRuns := listExecutions(t, token, "tick")
	if len(tickRuns) != 1 || tickRuns[0].Status != "done" || len(tickRuns[0].Created) != 1 || tickRuns[0].Created[0] != notes[0].ID {
		t.Fatalf("tick %+v", tickRuns)
	}

	clearFunctionWork(t, orgID)
}

func functionWorkflow(name, label, event, object, function, when string) map[string]any {
	body := map[string]any{
		"name":  name,
		"label": label,
		"actions": []any{
			map[string]any{"type": "function", "name": function},
		},
	}
	if event == "" {
		body["every"] = "1m"
		return body
	}
	body["event"] = event
	body["object"] = object
	if when != "" {
		body["conditions"] = []any{map[string]any{"field": "name", "op": "eq", "value": when}}
	}
	return body
}

func createdEventID(t *testing.T, token, recordID string) string {
	t.Helper()
	created := eventsNamed(eventsFor(listEvents(t, token, "record="+recordID), recordID), "record.created")
	if len(created) != 1 {
		t.Fatalf("created events %d for %s", len(created), recordID)
	}
	return created[0].ID
}

func executionFor(executions []executionView, eventID string) (executionView, bool) {
	for _, execution := range executions {
		if execution.EventID == eventID {
			return execution, true
		}
	}
	return executionView{}, false
}

func pendingAutomation(t *testing.T, jobs []jobView) jobView {
	t.Helper()
	var pending []jobView
	for _, job := range jobs {
		if job.Kind == "automation" && job.Status == "pending" {
			pending = append(pending, job)
		}
	}
	if len(pending) != 1 {
		t.Fatalf("pending jobs %+v", jobs)
	}
	return pending[0]
}

func clearFunctionWork(t *testing.T, orgID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `
		delete from automation_definitions where organization_id = $1::uuid
	`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `
		delete from jobs where organization_id = $1::uuid and status <> 'done'
	`, orgID); err != nil {
		t.Fatal(err)
	}
	var due int
	if err := testPool.QueryRow(ctx, `
		select count(*) from jobs
		where organization_id = $1::uuid and status = 'pending' and run_at <= now()
	`, orgID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if due != 0 {
		t.Fatalf("due jobs %d", due)
	}
}
