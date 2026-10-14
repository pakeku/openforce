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

func TestApprovalWaitsForTheMember(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer bad.Close()

	token := newOwner(t, "Approval", "approval-owner@example.com")
	orgID := orgOf(t, token)
	const memberEmail = "approval-member@example.com"
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `delete from automation_definitions where organization_id = $1::uuid`, orgID)
		_, _ = testPool.Exec(ctx, `delete from jobs where organization_id = $1::uuid and status <> 'done'`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "task",
		"label": "Task",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", memberEmail)
	if err != nil {
		t.Fatal(err)
	}
	bystander, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Ned", "approval-bystander@example.com")
	if err != nil {
		t.Fatal(err)
	}
	foreign := newOwner(t, "Approval Other", "approval-foreign@example.com")
	_ = foreign

	rec := apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "unknown_user", "label": "Unknown", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "approval", "user": "nobody@example.com"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "not found" {
		t.Fatalf("unknown email %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "foreign_user", "label": "Foreign", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "approval", "user": "approval-foreign@example.com"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "not found" {
		t.Fatalf("other org %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "blank_user", "label": "Blank", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "approval", "user": " "}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "user is required" {
		t.Fatalf("user %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "extra", "label": "Extra", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "approval", "user": memberEmail, "object": "task"}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("extra %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "twice", "label": "Twice", "event": "record.created", "object": "company",
		"actions": []any{
			map[string]any{"type": "approval", "user": memberEmail},
			map[string]any{"type": "approval", "user": memberEmail},
		},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("second approval %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "else_wait", "label": "Else wait", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Ok"}}},
		"else":    []any{map[string]any{"type": "approval", "user": memberEmail}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("else approval %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "patch_me", "label": "Patch", "event": "record.created", "object": "company",
		"actions": []any{map[string]any{"type": "approval", "user": memberEmail}},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPatch, "/api/automations/patch_me", token, map[string]any{
		"version": 1,
		"actions": []any{
			map[string]any{"type": "approval", "user": memberEmail},
			map[string]any{"type": "approval", "user": memberEmail},
		},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("patch second %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPatch, "/api/automations/patch_me", token, map[string]any{
		"version": 1,
		"else":    []any{map[string]any{"type": "approval", "user": memberEmail}},
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid action" {
		t.Fatalf("patch else %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodDelete, "/api/automations/patch_me", token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = apiDo(t, http.MethodPost, "/api/automations", token, map[string]any{
		"name": "clock", "label": "Clock", "every": "1d",
		"actions": []any{map[string]any{"type": "approval", "user": memberEmail}},
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
	rec = apiDo(t, http.MethodGet, "/api/approvals", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	requireExactForbidden(t, apiDo(t, http.MethodGet, "/api/approvals", key.Token, nil))
	requireExactForbidden(t, apiDo(t, http.MethodPost, "/api/approvals/"+uuid.NewString(), key.Token, map[string]any{"decision": "approve"}))
	if len(listApprovals(t, member)) != 0 || len(listApprovals(t, token)) != 0 || len(listApprovals(t, bystander)) != 0 {
		t.Fatal("approvals existed before the job")
	}

	postApprovalWorkflow(t, token, "review", "Review", memberEmail, []any{
		map[string]any{"type": "notification", "user": memberEmail, "title": "Review", "body": ""},
		map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Earlier"}},
		map[string]any{"type": "approval", "user": memberEmail},
		map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Later"}},
	}, nil)
	postApprovalWorkflow(t, token, "decline", "Decline", memberEmail, []any{
		map[string]any{"type": "approval", "user": memberEmail},
		map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Kept"}},
	}, []any{
		map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Declined"}},
	})
	postApprovalWorkflow(t, token, "quiet", "", memberEmail, []any{
		map[string]any{"type": "approval", "user": memberEmail},
		map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Too big"}},
	}, nil)
	postApprovalWorkflow(t, token, "broken", "Broken", memberEmail, []any{
		map[string]any{"type": "approval", "user": memberEmail},
		map[string]any{"type": "http", "url": bad.URL},
	}, nil)
	postApprovalWorkflow(t, token, "missed", "Matchbox", memberEmail, []any{
		map[string]any{"type": "approval", "user": memberEmail},
		map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Should wait"}},
	}, []any{
		map[string]any{"type": "create", "object": "task", "data": map[string]any{"name": "Ran else"}},
	})

	reviewRecord := postRecord(t, token, "company", map[string]any{"name": "Review", "employees": 5, "industry": "SaaS"})
	declineRecord := postRecord(t, token, "company", map[string]any{"name": "Decline", "employees": 5, "industry": "SaaS"})
	brokenRecord := postRecord(t, token, "company", map[string]any{"name": "Broken", "employees": 5, "industry": "SaaS"})
	if len(tasksNamed(listRecords(t, token, "task", ""), "Earlier")) != 0 {
		t.Fatal("the record request ran the workflow")
	}
	runOnce(t)

	tasks := listRecords(t, token, "task", "")
	earlier := tasksNamed(tasks, "Earlier")
	// Review and Broken miss decline, so else creates those two before the matching record is rejected.
	declinedBefore := tasksNamed(tasks, "Declined")
	if len(earlier) != 1 || len(tasksNamed(tasks, "Later")) != 0 || len(tasksNamed(tasks, "Kept")) != 0 || len(declinedBefore) != 2 || len(tasksNamed(tasks, "Too big")) != 0 || len(tasksNamed(tasks, "Should wait")) != 0 || len(tasksNamed(tasks, "Ran else")) != 3 {
		t.Fatalf("tasks after work %#v", tasks)
	}
	if dueJobs(t, orgID) != 0 {
		t.Fatalf("work left %d due jobs", dueJobs(t, orgID))
	}
	notes := listNotifications(t, member)
	if len(notes) != 1 || notes[0].Title != "Review" || notes[0].Body != "" || len(listNotifications(t, token)) != 0 {
		t.Fatalf("notification %+v", notes)
	}
	reviewEvent := createdEventID(t, token, reviewRecord.ID)
	reviewExec := listExecutions(t, token, "review")
	waiting, ok := executionFor(reviewExec, reviewEvent)
	if !ok || waiting.Status != "waiting" || waiting.Path != "" || len(waiting.Created) != 1 || waiting.Created[0] != earlier[0].ID || len(reviewExec) != 3 || skippedExecutions(reviewExec) != 2 {
		t.Fatalf("waiting %+v executions %+v", waiting, reviewExec)
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/review/executions", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), `"path"`) {
		t.Fatalf("waiting path %s", rec.Body.String())
	}
	declineEvent := createdEventID(t, token, declineRecord.ID)
	paused, ok := executionFor(listExecutions(t, token, "decline"), declineEvent)
	if !ok || paused.Status != "waiting" || paused.Path != "" || len(paused.Created) != 0 {
		t.Fatalf("decline waiting %+v", paused)
	}
	quiet := listExecutions(t, token, "quiet")
	if len(quiet) != 3 {
		t.Fatalf("quiet executions %+v", quiet)
	}
	for _, execution := range quiet {
		if execution.Status != "skipped" || execution.Path != "" || len(execution.Created) != 0 {
			t.Fatalf("quiet %+v", execution)
		}
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/quiet/executions", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), `"path"`) {
		t.Fatalf("skipped path %s", rec.Body.String())
	}
	for _, execution := range listExecutions(t, token, "missed") {
		if execution.Status != "done" || execution.Path != "else" || len(execution.Created) != 1 {
			t.Fatalf("miss else %+v", execution)
		}
	}
	if len(listExecutions(t, token, "missed")) != 3 {
		t.Fatal("a miss waited")
	}
	mu.Lock()
	seen := hits
	mu.Unlock()
	if seen != 0 {
		t.Fatalf("http ran before approval: %d", seen)
	}

	rows := listApprovals(t, member)
	if len(rows) != 3 || rows[0].CreatedAt.Before(rows[1].CreatedAt) || rows[1].CreatedAt.Before(rows[2].CreatedAt) {
		t.Fatalf("order %+v", rows)
	}
	reviewApproval := approvalNamed(t, rows, "review")
	declineApproval := approvalNamed(t, rows, "decline")
	brokenApproval := approvalNamed(t, rows, "broken")
	if _, err := uuid.Parse(reviewApproval.ID); err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodGet, "/api/approvals", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var raw struct {
		Approvals []map[string]any `json:"approvals"`
	}
	mustJSON(t, rec, &raw)
	if len(raw.Approvals) != 3 {
		t.Fatalf("shape count %d", len(raw.Approvals))
	}
	for _, item := range raw.Approvals {
		if len(item) != 3 || item["id"] == "" || item["name"] == "" || item["created_at"] == "" {
			t.Fatalf("shape %#v", item)
		}
	}
	if len(listApprovals(t, token)) != 0 || len(listApprovals(t, bystander)) != 0 {
		t.Fatal("another user saw the approval")
	}
	requireExactForbidden(t, apiDo(t, http.MethodPost, "/api/approvals/"+reviewApproval.ID, token, map[string]any{"decision": "approve"}))
	requireExactForbidden(t, apiDo(t, http.MethodPost, "/api/approvals/"+reviewApproval.ID, bystander, map[string]any{"decision": "reject"}))
	rec = apiDo(t, http.MethodPost, "/api/approvals/not-a-uuid", member, map[string]any{"decision": "approve"})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPost, "/api/approvals/"+uuid.NewString(), member, map[string]any{"decision": "approve"})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPost, "/api/approvals/"+reviewApproval.ID, member, map[string]any{"decision": "hold"})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid decision" {
		t.Fatalf("decision %s", rec.Body.String())
	}
	if executionForStatus(t, token, "review", reviewEvent) != "waiting" {
		t.Fatal("invalid decision moved the execution")
	}

	runOnce(t)
	if len(tasksNamed(listRecords(t, token, "task", ""), "Later")) != 0 || executionForStatus(t, token, "review", reviewEvent) != "waiting" {
		t.Fatal("a second pass continued the workflow")
	}

	rec = apiDo(t, http.MethodPost, "/api/approvals/"+reviewApproval.ID, member, map[string]any{"decision": "approve"})
	requireStatus(t, rec, http.StatusOK)
	var decided executionView
	mustJSON(t, rec, &decided)
	later := tasksNamed(listRecords(t, token, "task", ""), "Later")
	if decided.Status != "done" || decided.Path != "actions" || len(decided.Created) != 2 || decided.Created[0] != earlier[0].ID || len(later) != 1 || decided.Created[1] != later[0].ID {
		t.Fatalf("approved %+v later %#v", decided, later)
	}
	if len(tasksNamed(listRecords(t, token, "task", ""), "Earlier")) != 1 || len(listNotifications(t, member)) != 1 {
		t.Fatal("approve repeated the earlier steps")
	}
	if !approvalAbsent(listApprovals(t, member), "review") {
		t.Fatal("approved row stayed pending")
	}
	rec = apiDo(t, http.MethodPost, "/api/approvals/"+reviewApproval.ID, member, map[string]any{"decision": "approve"})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "already decided" {
		t.Fatalf("second decision %s", rec.Body.String())
	}
	if len(tasksNamed(listRecords(t, token, "task", ""), "Later")) != 1 {
		t.Fatal("the second decision ran again")
	}
	if automationJobs(t, token, reviewEvent) != 5 {
		t.Fatal("approve enqueued another automation job")
	}

	rec = apiDo(t, http.MethodPost, "/api/approvals/"+declineApproval.ID, member, map[string]any{"decision": "reject"})
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &decided)
	declined := tasksNamed(listRecords(t, token, "task", ""), "Declined")
	if decided.Status != "done" || decided.Path != "else" || len(decided.Created) != 1 || len(declined) != len(declinedBefore)+1 || !taskID(declined, decided.Created[0]) || taskID(declinedBefore, decided.Created[0]) || len(tasksNamed(listRecords(t, token, "task", ""), "Kept")) != 0 {
		t.Fatalf("rejected %+v tasks %#v", decided, listRecords(t, token, "task", ""))
	}
	stored, ok := executionFor(listExecutions(t, token, "decline"), declineEvent)
	if !ok || stored.Status != "done" || stored.Path != "else" || len(stored.Created) != 1 || stored.Created[0] != decided.Created[0] {
		t.Fatalf("decline execution %+v", stored)
	}
	rec = apiDo(t, http.MethodPost, "/api/approvals/"+declineApproval.ID, member, map[string]any{"decision": "reject"})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "already decided" {
		t.Fatalf("second reject %s", rec.Body.String())
	}

	clearPending(t, orgID)
	if dueJobs(t, orgID) != 0 {
		t.Fatal("left a due job")
	}
	rec = apiDo(t, http.MethodPost, "/api/approvals/"+brokenApproval.ID, member, map[string]any{"decision": "approve"})
	requireStatus(t, rec, http.StatusBadGateway)
	if errorOf(t, rec) != "http status 500" {
		t.Fatalf("action error %s", rec.Body.String())
	}
	brokenEvent := createdEventID(t, token, brokenRecord.ID)
	if executionForStatus(t, token, "broken", brokenEvent) != "waiting" {
		t.Fatal("a failed action left the execution")
	}
	if approvalAbsent(listApprovals(t, member), "broken") {
		t.Fatal("a failed action dropped the approval")
	}
	mu.Lock()
	seen = hits
	mu.Unlock()
	if seen != 1 || dueJobs(t, orgID) != 0 || len(listJobs(t, token, "event="+brokenEvent+"&status=pending")) != 0 {
		t.Fatalf("failed action hits %d due %d", seen, dueJobs(t, orgID))
	}
	if automationJobs(t, token, brokenEvent) != 5 {
		t.Fatal("a failed action enqueued another automation job")
	}

	rec = apiDo(t, http.MethodPost, "/api/approvals/"+brokenApproval.ID, member, map[string]any{"decision": "reject"})
	requireStatus(t, rec, http.StatusOK)
	decided = executionView{}
	mustJSON(t, rec, &decided)
	if decided.Status != "rejected" || decided.Path != "" {
		t.Fatalf("empty else %+v", decided)
	}
	rejected, ok := executionFor(listExecutions(t, token, "broken"), brokenEvent)
	if !ok || rejected.Status != "rejected" || rejected.Path != "" || len(rejected.Created) != 0 {
		t.Fatalf("rejected execution %+v", rejected)
	}
	rec = apiDo(t, http.MethodGet, "/api/automations/broken/executions", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), `"path"`) {
		t.Fatalf("rejected path %s", rec.Body.String())
	}
	mu.Lock()
	seen = hits
	mu.Unlock()
	if seen != 1 || !approvalAbsent(listApprovals(t, member), "broken") {
		t.Fatalf("reject reran http %d or left the row", seen)
	}
	rec = apiDo(t, http.MethodPost, "/api/approvals/"+brokenApproval.ID, member, map[string]any{"decision": "approve"})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "already decided" {
		t.Fatalf("after reject %s", rec.Body.String())
	}

	clearPending(t, orgID)
	if dueJobs(t, orgID) != 0 {
		t.Fatal("left a due job")
	}
}

func postApprovalWorkflow(t *testing.T, token, name, when, email string, actions, elseActions []any) {
	t.Helper()
	body := map[string]any{
		"name": name, "label": name, "event": "record.created", "object": "company",
		"actions": actions,
	}
	if when == "" {
		body["conditions"] = []any{map[string]any{"field": "employees", "op": "gte", "value": 1000}}
	} else {
		body["conditions"] = []any{map[string]any{"field": "name", "op": "eq", "value": when}}
	}
	if elseActions != nil {
		body["else"] = elseActions
	}
	rec := apiDo(t, http.MethodPost, "/api/automations", token, body)
	requireStatus(t, rec, http.StatusCreated)
	if email == "" {
		t.Fatal("email")
	}
}

func runOnce(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", testURL)
	t.Setenv("REDIS_URL", "")
	code, out := runWorkCaptured(t)
	if code != 0 {
		t.Fatalf("work %d\n%s", code, out)
	}
}

func listApprovals(t *testing.T, token string) []approvalView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/approvals", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Approvals []approvalView `json:"approvals"`
	}
	mustJSON(t, rec, &body)
	if body.Approvals == nil {
		t.Fatal("null approvals")
	}
	return body.Approvals
}

func approvalNamed(t *testing.T, rows []approvalView, name string) approvalView {
	t.Helper()
	for _, row := range rows {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("missing approval %s in %+v", name, rows)
	return approvalView{}
}

func approvalAbsent(rows []approvalView, name string) bool {
	for _, row := range rows {
		if row.Name == name {
			return false
		}
	}
	return true
}

func taskID(tasks []recordView, id string) bool {
	for _, task := range tasks {
		if task.ID == id {
			return true
		}
	}
	return false
}

func tasksNamed(tasks []recordView, name string) []recordView {
	named := []recordView{}
	for _, task := range tasks {
		if task.Data["name"] == name {
			named = append(named, task)
		}
	}
	return named
}

func executionForStatus(t *testing.T, token, name, eventID string) string {
	t.Helper()
	execution, ok := executionFor(listExecutions(t, token, name), eventID)
	if !ok {
		t.Fatalf("no execution for %s", eventID)
	}
	return execution.Status
}

func automationJobs(t *testing.T, token, eventID string) int {
	t.Helper()
	n := 0
	for _, job := range listJobs(t, token, "event="+eventID) {
		if job.Kind != "automation" {
			continue
		}
		n++
		if job.Status != "done" {
			t.Fatalf("automation job %+v", job)
		}
	}
	return n
}

func dueJobs(t *testing.T, orgID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `
		select count(*) from jobs
		where organization_id = $1::uuid and status = 'pending' and run_at <= now()
	`, orgID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func clearPending(t *testing.T, orgID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `
		delete from jobs
		where organization_id = $1::uuid and status <> 'done'
	`, orgID); err != nil {
		t.Fatal(err)
	}
}

func skippedExecutions(executions []executionView) int {
	n := 0
	for _, execution := range executions {
		if execution.Status == "skipped" {
			n++
		}
	}
	return n
}
