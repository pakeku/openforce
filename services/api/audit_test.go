package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAuditFollowsStoredEvents(t *testing.T) {
	token := newOwner(t, "Audit", "audit-owner@example.com")
	ownerID := userOf(t, token)
	postObject(t, token, companyObject())
	acme := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	rec := apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": 1,
		"data":    map[string]any{"industry": "Finance"},
	})
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": 2,
		"data":    map[string]any{"industry": "Finance"},
	})
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodDelete, "/api/objects/company/"+acme.ID, token, nil)
	requireStatus(t, rec, http.StatusNoContent)

	if len(listAudit(t, token, acme.ID)) != 0 {
		t.Fatal("the record request wrote audit entries")
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+uuid.NewString()+"/audit", token, nil)
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodGet, "/api/objects/company/not-a-uuid/audit", token, nil)
	requireStatus(t, rec, http.StatusNotFound)

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgOf(t, token), "Mia", "audit-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+acme.ID+"/audit", member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if errorOf(t, rec) != "forbidden" || strings.Contains(rec.Body.String(), "Acme") {
		t.Fatalf("member body %s", rec.Body.String())
	}

	n, err := (&server{pool: testPool}).deliverAudit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("delivered %d", n)
	}
	entries := listAudit(t, token, acme.ID)
	if len(entries) != 5 {
		t.Fatalf("entries %d %#v", len(entries), entries)
	}
	if entries[0].Field == nil || *entries[0].Field != "name" || entries[0].Before != nil || entries[0].After != "Acme" {
		t.Fatalf("name %#v", entries[0])
	}
	if entries[1].Field == nil || *entries[1].Field != "employees" || entries[1].After != float64(10) {
		t.Fatalf("employees %#v", entries[1])
	}
	if entries[2].Field == nil || *entries[2].Field != "industry" || entries[2].Before != nil || entries[2].After != "SaaS" {
		t.Fatalf("industry create %#v", entries[2])
	}
	if entries[3].Field == nil || *entries[3].Field != "industry" || entries[3].Before != "SaaS" || entries[3].After != "Finance" {
		t.Fatalf("industry update %#v", entries[3])
	}
	if entries[4].Name != "record.deleted" || entries[4].Field != nil || entries[4].Before != nil || entries[4].After != nil {
		t.Fatalf("delete %#v", entries[4])
	}
	events := eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	byEvent := map[string]eventView{}
	for _, ev := range events {
		byEvent[ev.ID] = ev
	}
	for _, entry := range entries {
		ev, ok := byEvent[entry.EventID]
		if !ok || !entry.CreatedAt.Equal(ev.CreatedAt) || entry.ActorID != ownerID || entry.Source != "API" {
			t.Fatalf("entry %+v event %+v", entry, ev)
		}
	}
	again, err := (&server{pool: testPool}).deliverAudit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 || len(listAudit(t, token, acme.ID)) != 5 {
		t.Fatalf("second delivery %d", again)
	}

	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+acme.ID+"/audit", member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if strings.Contains(rec.Body.String(), "Acme") {
		t.Fatalf("member saw history %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", token, map[string]any{
		"role": "member", "can_read": true, "can_write": false,
	})
	requireStatus(t, rec, http.StatusOK)
	if listAudit(t, member, acme.ID)[0].After != "Acme" {
		t.Fatal("member with read could not see history")
	}

	other := newOwner(t, "Audit Other", "audit-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+acme.ID+"/audit", other, nil)
	requireStatus(t, rec, http.StatusNotFound)
	if strings.Contains(rec.Body.String(), "Acme") {
		t.Fatalf("other org saw %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+acme.ID, token, nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestWorkCommandDeliversAudit(t *testing.T) {
	token := newOwner(t, "AuditWork", "audit-work@example.com")
	postObject(t, token, companyObject())
	acme := postRecord(t, token, "company", map[string]any{"name": "Worked", "employees": 1, "industry": "SaaS"})
	if len(listAudit(t, token, acme.ID)) != 0 {
		t.Fatal("the record request wrote audit entries")
	}
	t.Setenv("DATABASE_URL", testURL)
	if code, out := runWorkCaptured(t); code != 0 {
		t.Fatalf("work %d\n%s", code, out)
	}
	entries := listAudit(t, token, acme.ID)
	if len(entries) != 3 || entries[0].After != "Worked" {
		t.Fatalf("entries %#v", entries)
	}
	if code, out := runWorkCaptured(t); code != 0 || !strings.Contains(out, "delivered 0") {
		t.Fatalf("second work %d\n%s", code, out)
	}
	if len(listAudit(t, token, acme.ID)) != 3 {
		t.Fatal("second work duplicated history")
	}
}

func TestWorkLoopDeliversAndReturns(t *testing.T) {
	token := newOwner(t, "WorkLoop", "work-loop@example.com")
	postObject(t, token, companyObject())
	t.Setenv("DATABASE_URL", testURL)
	t.Setenv("REDIS_URL", "")

	ctx, cancel := context.WithCancel(context.Background())
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	out := &lockedBuf{}
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(out, r)
		close(copyDone)
	}()
	codeCh := make(chan int, 1)
	go func() {
		codeCh <- workSession(ctx, false)
	}()

	var recordID string
	joined := false
	t.Cleanup(func() {
		cancel()
		if !joined {
			select {
			case <-codeCh:
			case <-time.After(5 * time.Second):
			}
		}
		os.Stdout = old
		_ = w.Close()
		<-copyDone
		if recordID == "" {
			return
		}
		_, _ = testPool.Exec(context.Background(), `
			update jobs j
			set status = 'done', updated_at = now()
			from events e
			where j.event_id = e.id
			  and e.record_id = $1::uuid
			  and j.status = 'pending'
			  and j.run_at <= now()
		`, recordID)
	})

	acme := postRecord(t, token, "company", map[string]any{"name": "Resident", "employees": 2, "industry": "SaaS"})
	recordID = acme.ID

	deadline := time.Now().Add(8 * time.Second)
	for {
		entries := listAudit(t, token, acme.ID)
		if len(entries) == 3 && entries[0].After == "Resident" && deliveredCount(out.String()) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not delivered\n%s", out.String())
		}
		select {
		case code := <-codeCh:
			joined = true
			t.Fatalf("loop returned %d before delivery\n%s", code, out.String())
		case <-time.After(30 * time.Millisecond):
		}
	}

	cancel()
	select {
	case code := <-codeCh:
		joined = true
		if code != 0 {
			t.Fatalf("work loop %d\n%s", code, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("work loop did not return")
	}

	var due int
	if err := testPool.QueryRow(context.Background(), `
		select count(*)
		from jobs j
		join events e on e.id = j.event_id
		where e.record_id = $1::uuid and j.status = 'pending' and j.run_at <= now()
	`, acme.ID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if due != 0 {
		t.Fatalf("left %d due jobs", due)
	}
}

func deliveredCount(out string) int {
	best := 0
	for _, line := range strings.Split(out, "\n") {
		var n int
		if _, err := fmt.Sscanf(line, "delivered %d", &n); err == nil && n > best {
			best = n
		}
	}
	return best
}

func listAudit(t *testing.T, token, id string) []auditView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/objects/company/"+id+"/audit", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Entries []auditView `json:"entries"`
	}
	mustJSON(t, rec, &body)
	if body.Entries == nil {
		t.Fatal("null entries")
	}
	return body.Entries
}

func runWorkCaptured(t *testing.T) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := runWork([]string{"-once"})
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return code, buf.String()
}
