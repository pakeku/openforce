package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestLoopVisitsMatchingRecords(t *testing.T) {
	token := newOwner(t, "Loop Visit", "loop-visit@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `delete from jobs where organization_id = $1::uuid`, orgID)
		_, _ = testPool.Exec(ctx, `delete from automation_definitions where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "task",
		"label": "Task",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "status", "type": "text"},
			{"name": "company", "type": "relation", "to": "company"},
		},
	})
	postObject(t, token, map[string]any{
		"name":  "note",
		"label": "Note",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})
	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "related", "label": "Related", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{
			"type": "loop", "object": "task", "limit": 2,
			"where": []any{
				map[string]any{"field": "status", "op": "eq", "value": "open"},
				map[string]any{"field": "company", "op": "eq", "value": map[string]any{"record": true}},
			},
			"actions": []any{
				map[string]any{"type": "update", "data": map[string]any{"status": "done"}},
				map[string]any{"type": "create", "object": "note", "data": map[string]any{"name": map[string]any{"field": "name"}}},
				map[string]any{"type": "email", "to": "ada@example.com", "subject": "closed", "body": map[string]any{"field": "name"}},
			},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var saved automationView
	mustJSON(t, rec, &saved)
	if saved.Actions[0].Limit == nil || *saved.Actions[0].Limit != 2 || len(saved.Actions[0].Where) != 2 || !isRecordRef(saved.Actions[0].Where[1].Value) {
		t.Fatalf("saved %#v", saved.Actions[0])
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/related", token, nil)
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &saved)
	if len(saved.Actions) != 1 || saved.Actions[0].Object != "task" || len(saved.Actions[0].Actions) != 3 {
		t.Fatalf("round trip %#v", saved.Actions)
	}

	companyA := postRecord(t, token, "company", map[string]any{"name": "A"}).ID
	companyB := postRecord(t, token, "company", map[string]any{"name": "B"}).ID
	companyC := postRecord(t, token, "company", map[string]any{"name": "C"}).ID
	names := map[string]string{}
	add := func(company, name, status string) string {
		t.Helper()
		view := postRecord(t, token, "task", map[string]any{"name": name, "status": status, "company": company})
		names[view.ID] = name
		return view.ID
	}
	add(companyA, "A1", "open")
	add(companyA, "A2", "open")
	add(companyA, "A3", "open")
	gone := add(companyA, "Gone", "open")
	add(companyB, "B1", "open")
	closed := add(companyC, "C1", "closed")
	rec = apiDo(t, http.MethodDelete, "/api/objects/task/"+gone, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	openA := tasksWhere(t, orgID, companyA, "open")
	if len(openA) != 3 {
		t.Fatalf("open A %v", openA)
	}

	runOnce(t)
	if code, out := runWorkCaptured(t); code != 0 || !strings.Contains(out, "delivered 0") {
		t.Fatalf("second drain %d %s", code, out)
	}
	if dueJobs(t, orgID) != 0 {
		t.Fatal("left a due job")
	}
	for i, id := range openA {
		status := getRecord(t, token, "task", id).Data["status"]
		want := "open"
		if i < 2 {
			want = "done"
		}
		if status != want {
			t.Fatalf("task %s status %v", names[id], status)
		}
	}
	bDone := tasksWhere(t, orgID, companyB, "done")
	if len(bDone) != 1 || names[bDone[0]] != "B1" {
		t.Fatalf("B %v", bDone)
	}
	if getRecord(t, token, "task", closed).Data["status"] != "closed" {
		t.Fatal("closed task changed")
	}
	_, deleted, raw := storedRecord(t, gone)
	if !deleted || raw["status"] != "open" {
		t.Fatalf("deleted task %+v deleted %v", raw, deleted)
	}

	executions := listFlowExecutions(t, token, "related")
	if len(executions) != 3 {
		t.Fatalf("executions %+v", executions)
	}
	var two, one, none int
	for _, execution := range executions {
		if execution.Status != "done" || execution.Path != "actions" {
			t.Fatalf("execution %+v", execution)
		}
		switch len(execution.Created) {
		case 2:
			two++
			if noteName(t, token, execution.Created[0]) != names[openA[0]] || noteName(t, token, execution.Created[1]) != names[openA[1]] {
				t.Fatalf("notes %s %s", noteName(t, token, execution.Created[0]), noteName(t, token, execution.Created[1]))
			}
		case 1:
			one++
			if noteName(t, token, execution.Created[0]) != "B1" {
				t.Fatalf("B note %s", noteName(t, token, execution.Created[0]))
			}
		case 0:
			none++
		default:
			t.Fatalf("created %+v", execution.Created)
		}
	}
	if two != 1 || one != 1 || none != 1 {
		t.Fatalf("created counts two %d one %d none %d", two, one, none)
	}
	bodies := map[string]int{}
	for _, message := range listStoredEmails(t, token) {
		if message.To != "ada@example.com" || message.Subject != "closed" {
			t.Fatalf("email %+v", message)
		}
		bodies[message.Body]++
	}
	if bodies[names[openA[0]]] != 1 || bodies[names[openA[1]]] != 1 || bodies["B1"] != 1 || len(bodies) != 3 {
		t.Fatalf("bodies %v", bodies)
	}
}

func TestLoopTakesTheOtherPath(t *testing.T) {
	token := newOwner(t, "Loop Else", "loop-else@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `delete from jobs where organization_id = $1::uuid`, orgID)
		_, _ = testPool.Exec(ctx, `delete from automation_definitions where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "task",
		"label": "Task",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "status", "type": "text"},
			{"name": "company", "type": "relation", "to": "company"},
		},
	})
	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "branch", "label": "Branch", "event": "record.created", "object": "company",
		"conditions": []any{map[string]any{"field": "name", "op": "eq", "value": "Acme"}},
		"actions":    []any{map[string]any{"type": "update", "data": map[string]any{"industry": "Finance"}}},
		"else": []any{map[string]any{
			"type": "loop", "object": "task",
			"where":   []any{map[string]any{"field": "company", "op": "eq", "value": map[string]any{"record": true}}},
			"actions": []any{map[string]any{"type": "update", "data": map[string]any{"status": "closed"}}},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var saved automationView
	mustJSON(t, rec, &saved)
	if saved.Else[0].Limit == nil || *saved.Else[0].Limit != loopDefaultLimit {
		t.Fatalf("default limit %#v", saved.Else[0].Limit)
	}

	other := postRecord(t, token, "company", map[string]any{"name": "Other"})
	otherTask := postRecord(t, token, "task", map[string]any{"name": "Follow", "status": "open", "company": other.ID})
	runOnce(t)
	if getRecord(t, token, "task", otherTask.ID).Data["status"] != "closed" {
		t.Fatal("else did not close the task")
	}
	if getRecord(t, token, "company", other.ID).Data["industry"] != nil {
		t.Fatal("else updated the company")
	}

	acme := postRecord(t, token, "company", map[string]any{"name": "Acme"})
	acmeTask := postRecord(t, token, "task", map[string]any{"name": "Keep", "status": "open", "company": acme.ID})
	runOnce(t)
	if getRecord(t, token, "company", acme.ID).Data["industry"] != "Finance" {
		t.Fatal("match did not update the company")
	}
	if getRecord(t, token, "task", acmeTask.ID).Data["status"] != "open" {
		t.Fatal("match ran the loop")
	}
	if code, out := runWorkCaptured(t); code != 0 || !strings.Contains(out, "delivered 0") {
		t.Fatalf("second drain %d %s", code, out)
	}
	if dueJobs(t, orgID) != 0 {
		t.Fatal("left a due job")
	}
}

func TestLoopDoesNotEnqueueItself(t *testing.T) {
	token := newOwner(t, "Loop Self", "loop-self@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `delete from jobs where organization_id = $1::uuid`, orgID)
		_, _ = testPool.Exec(ctx, `delete from automation_definitions where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, map[string]any{
		"name":  "task",
		"label": "Task",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "status", "type": "text"},
		},
	})
	postObject(t, token, map[string]any{
		"name":  "log",
		"label": "Log",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})
	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "close", "label": "Close", "event": "record.updated", "object": "task",
		"actions": []any{map[string]any{
			"type": "loop", "object": "task",
			"where":   []any{map[string]any{"field": "status", "op": "eq", "value": "open"}},
			"actions": []any{map[string]any{"type": "update", "data": map[string]any{"status": "done"}}},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "echo", "label": "Echo", "event": "record.updated", "object": "task",
		"actions": []any{map[string]any{"type": "create", "object": "log", "data": map[string]any{"name": "seen"}}},
	})
	requireStatus(t, rec, http.StatusCreated)
	first := postRecord(t, token, "task", map[string]any{"name": "One", "status": "open"})
	second := postRecord(t, token, "task", map[string]any{"name": "Two", "status": "open"})
	rec = apiDo(t, http.MethodPatch, "/api/objects/task/"+first.ID, token, map[string]any{
		"version": first.Version,
		"data":    map[string]any{"name": "Renamed"},
	})
	requireStatus(t, rec, http.StatusOK)
	runOnce(t)
	if getRecord(t, token, "task", first.ID).Data["status"] != "done" || getRecord(t, token, "task", second.ID).Data["status"] != "done" {
		t.Fatal("open tasks stayed open")
	}
	if len(listFlowExecutions(t, token, "close")) != 1 {
		t.Fatalf("close ran %d times", len(listFlowExecutions(t, token, "close")))
	}
	if len(listFlowExecutions(t, token, "echo")) != 3 {
		t.Fatalf("echo ran %d times", len(listFlowExecutions(t, token, "echo")))
	}
	if len(listRecords(t, token, "log", "")) != 3 {
		t.Fatalf("logs %d", len(listRecords(t, token, "log", "")))
	}
	if code, out := runWorkCaptured(t); code != 0 || !strings.Contains(out, "delivered 0") {
		t.Fatalf("second drain %d %s", code, out)
	}
	if dueJobs(t, orgID) != 0 {
		t.Fatal("left a due job")
	}
}

func TestLoopRejectsUnboundedShapes(t *testing.T) {
	token := newOwner(t, "Loop Shape", "loop-shape@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `delete from jobs where organization_id = $1::uuid`, orgID)
		_, _ = testPool.Exec(ctx, `delete from automation_definitions where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "task",
		"label": "Task",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "status", "type": "text"},
		},
	})
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "loop-shape-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	inner := []any{map[string]any{"type": "update", "data": map[string]any{"status": "done"}}}
	base := func(actions []any) map[string]any {
		return map[string]any{
			"name": "again", "label": "Again", "event": "record.created", "object": "company",
			"actions": actions,
		}
	}
	loop := func(extra map[string]any) map[string]any {
		action := map[string]any{"type": "loop", "object": "task", "actions": inner}
		for key, value := range extra {
			action[key] = value
		}
		return base([]any{action})
	}
	rec := apiDo(t, http.MethodPost, "/api/automations", member, loop(nil))
	requireExactForbidden(t, rec)

	rejects := []struct {
		body   map[string]any
		status int
		err    string
	}{
		{base([]any{map[string]any{"type": "loop", "actions": inner}}), http.StatusBadRequest, "invalid name"},
		{loop(map[string]any{"limit": 0}), http.StatusBadRequest, "invalid limit"},
		{loop(map[string]any{"limit": 51}), http.StatusBadRequest, "invalid limit"},
		{loop(map[string]any{"limit": -3}), http.StatusBadRequest, "invalid limit"},
		{loop(map[string]any{"object": "missing"}), http.StatusNotFound, "not found"},
		{loop(map[string]any{"actions": []any{}}), http.StatusBadRequest, "actions are required"},
		{loop(map[string]any{"actions": []any{map[string]any{"type": "loop", "object": "task", "actions": inner}}}), http.StatusBadRequest, "invalid action"},
		{loop(map[string]any{"actions": []any{map[string]any{"type": "approval", "user": "loop-shape@example.com"}}}), http.StatusBadRequest, "invalid action"},
		{loop(map[string]any{"where": []any{map[string]any{"field": "missing", "op": "eq", "value": "x"}}}), http.StatusBadRequest, "unknown field missing"},
		{loop(map[string]any{"where": []any{map[string]any{"field": "status", "op": "gt", "value": "a"}}}), http.StatusBadRequest, "invalid operator for status"},
		{loop(map[string]any{"where": []any{map[string]any{"field": "status", "op": "eq", "value": map[string]any{"field": "missing"}}}}), http.StatusBadRequest, "unknown field missing"},
		{base([]any{map[string]any{
			"type": "loop", "object": "company",
			"where":   []any{map[string]any{"field": "employees", "op": "eq", "value": map[string]any{"record": true}}},
			"actions": []any{map[string]any{"type": "update", "data": map[string]any{"name": "Acme"}}},
		}}), http.StatusBadRequest, "invalid value for employees"},
		{map[string]any{
			"name": "clock", "label": "Clock", "every": "1m",
			"actions": []any{map[string]any{"type": "loop", "object": "task", "actions": inner}},
		}, http.StatusBadRequest, "invalid action"},
	}
	for _, item := range rejects {
		rec = apiDo(t, http.MethodPost, "/api/automations", token, item.body)
		requireStatus(t, rec, item.status)
		if errorOf(t, rec) != item.err {
			t.Fatalf("%s: %s", item.err, rec.Body.String())
		}
	}

	rec = apiDo(t, http.MethodPost, "/api/automations", token, loop(map[string]any{"limit": 50}))
	requireStatus(t, rec, http.StatusCreated)
	var capped automationView
	mustJSON(t, rec, &capped)
	if capped.Actions[0].Limit == nil || *capped.Actions[0].Limit != 50 {
		t.Fatalf("cap %#v", capped.Actions[0].Limit)
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "plain", "label": "Plain", "event": "record.updated", "object": "task",
		"actions": []any{map[string]any{
			"type": "loop", "object": "task",
			"actions": []any{map[string]any{"type": "delete"}},
		}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodGet, "/api/automations/plain", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var plain automationView
	mustJSON(t, rec, &plain)
	if plain.Actions[0].Limit == nil || *plain.Actions[0].Limit != loopDefaultLimit || len(plain.Actions[0].Where) != 0 {
		t.Fatalf("omitted %#v", plain.Actions[0])
	}
}

func tasksWhere(t *testing.T, orgID, companyID, status string) []string {
	t.Helper()
	rows, err := testPool.Query(context.Background(), `
		select r.id::text
		from records r
		join object_definitions o on o.id = r.object_id
		where r.organization_id = $1::uuid
			and o.name = 'task'
			and r.deleted_at is null
			and r.data->>'company' = $2
			and r.data->>'status' = $3
		order by r.created_at, r.id
	`, orgID, companyID, status)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func listFlowExecutions(t *testing.T, token, name string) []executionView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/automations/"+name+"/executions", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Executions []executionView `json:"executions"`
	}
	mustJSON(t, rec, &body)
	return body.Executions
}

func listStoredEmails(t *testing.T, token string) []emailView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/emails", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Emails []emailView `json:"emails"`
	}
	mustJSON(t, rec, &body)
	return body.Emails
}

func noteName(t *testing.T, token, id string) string {
	t.Helper()
	name, _ := getRecord(t, token, "note", id).Data["name"].(string)
	return name
}
