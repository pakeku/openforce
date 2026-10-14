package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestJobsWaitAndThenDie(t *testing.T) {
	token := newOwner(t, "Jobs", "jobs-owner@example.com")
	postObject(t, token, companyObject())
	acme := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	if len(listAudit(t, token, acme.ID)) != 0 {
		t.Fatal("the record request wrote audit entries")
	}

	recordEvents := eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	var createdID, fieldID string
	wanted := map[string]bool{}
	for _, ev := range recordEvents {
		if ev.Name == "record.created" {
			createdID = ev.ID
		}
		if ev.Name == "field.changed" {
			wanted[ev.ID] = true
			if ev.Payload["field"] == "name" {
				fieldID = ev.ID
			}
		}
	}
	if createdID == "" || fieldID == "" || len(wanted) != 3 {
		t.Fatalf("events %s", namesOf(recordEvents))
	}
	for id := range wanted {
		jobs := listJobs(t, token, "event="+id)
		if len(jobs) != 1 || jobs[0].Kind != "audit" || jobs[0].Status != "pending" || jobs[0].Attempts != 0 || jobs[0].LastError != nil {
			t.Fatalf("pending jobs %+v", jobs)
		}
	}
	if len(listJobs(t, token, "event="+createdID)) != 0 {
		t.Fatal("record.created stored a job")
	}

	brokenID := uuid.NewString()
	if _, err := testPool.Exec(context.Background(), `
		insert into jobs (id, organization_id, event_id, kind)
		values ($1, $2, $3, 'broken')
	`, brokenID, orgOf(t, token), fieldID); err != nil {
		t.Fatal(err)
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(listAudit(t, token, acme.ID)) != 3 {
		t.Fatal("audit job did not write history")
	}
	for id := range wanted {
		jobs := listJobs(t, token, "event="+id+"&status=done")
		if len(jobs) != 1 || jobs[0].Attempts != 1 || jobs[0].LastError != nil {
			t.Fatalf("done jobs %+v", jobs)
		}
	}
	broken := findJob(t, listJobs(t, token, "event="+fieldID), brokenID)
	if broken.Status != "pending" || broken.Attempts != 1 || broken.LastError == nil || !strings.Contains(*broken.LastError, "unknown job kind broken") {
		t.Fatalf("first failure %+v", broken)
	}

	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	broken = findJob(t, listJobs(t, token, "event="+fieldID), brokenID)
	if broken.Status != "pending" || broken.Attempts != 1 {
		t.Fatalf("retried before the wait %+v", broken)
	}

	if _, err := testPool.Exec(context.Background(), `
		update jobs set attempts = 4, run_at = now() where id = $1
	`, brokenID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&server{pool: testPool}).deliverJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	broken = findJob(t, listJobs(t, token, "event="+fieldID+"&status=dead"), brokenID)
	if broken.Status != "dead" || broken.Attempts != 5 {
		t.Fatalf("dead job %+v", broken)
	}

	other := newOwner(t, "Jobs Other", "jobs-other@example.com")
	rec := apiDo(t, http.MethodGet, "/api/jobs?status=dead", other, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), brokenID) {
		t.Fatalf("other org saw %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/jobs", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	rec = apiDo(t, http.MethodGet, "/api/jobs?status=later", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid status" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/jobs?limit=0", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid limit" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/jobs?event=not-a-uuid", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid event" {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func listJobs(t *testing.T, token, query string) []jobView {
	t.Helper()
	path := "/api/jobs"
	if query != "" {
		path += "?" + query
	}
	rec := apiDo(t, http.MethodGet, path, token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Jobs []jobView `json:"jobs"`
	}
	mustJSON(t, rec, &body)
	if body.Jobs == nil {
		t.Fatal("null jobs")
	}
	return body.Jobs
}

func findJob(t *testing.T, jobs []jobView, id string) jobView {
	t.Helper()
	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	t.Fatalf("missing job %s", id)
	return jobView{}
}
