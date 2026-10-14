package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEventsFollowCommittedChanges(t *testing.T) {
	token := newOwner(t, "Events", "events-owner@example.com")
	ownerID := userOf(t, token)
	object := postObject(t, token, companyObject())

	rec := apiDo(t, http.MethodPost, "/api/meta/objects", token, companyObject())
	requireStatus(t, rec, http.StatusConflict)
	rec = apiDo(t, http.MethodPost, "/api/objects/company", token, map[string]any{"industry": "SaaS"})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "missing required field name" {
		t.Fatalf("body %s", rec.Body.String())
	}

	createdUsers := eventsNamed(listEvents(t, token, "name=user.created"), "user.created")
	if len(createdUsers) != 1 {
		t.Fatalf("user.created %d", len(createdUsers))
	}
	if createdUsers[0].Source != "Admin" || createdUsers[0].ActorID != ownerID || createdUsers[0].ObjectID != nil || createdUsers[0].RecordID != nil {
		t.Fatalf("user event %+v", createdUsers[0])
	}
	if createdUsers[0].Payload["email"] != "events-owner@example.com" || createdUsers[0].Payload["user_id"] != ownerID {
		t.Fatalf("user payload %#v", createdUsers[0].Payload)
	}
	if createdUsers[0].CreatedAt.IsZero() {
		t.Fatal("missing created_at")
	}

	objects := eventsNamed(listEvents(t, token, "name=object.created"), "object.created")
	if len(objects) != 1 || objects[0].Source != "API" || objects[0].ActorID != ownerID {
		t.Fatalf("object events %+v", objects)
	}
	if objects[0].ObjectID == nil || *objects[0].ObjectID != object.ID || objects[0].Payload["name"] != "company" || objects[0].Payload["label"] != "Company" {
		t.Fatalf("object payload %+v", objects[0])
	}
	if len(eventsNamed(listEvents(t, token, ""), "record.created")) != 0 {
		t.Fatal("rejected create stored an event")
	}

	acme := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	got := eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	if namesOf(got) != "record.created,field.changed,field.changed,field.changed" {
		t.Fatalf("create events %s", namesOf(got))
	}
	created := got[0]
	if created.Source != "API" || created.ActorID != ownerID || created.ObjectID == nil || *created.ObjectID != object.ID {
		t.Fatalf("record.created %+v", created)
	}
	data, _ := created.Payload["data"].(map[string]any)
	if created.Payload["object"] != "company" || created.Payload["version"] != float64(1) || data["name"] != "Acme" {
		t.Fatalf("record payload %#v", created.Payload)
	}
	nameChange := fieldEvent(t, got, "name")
	employeesChange := fieldEvent(t, got, "employees")
	industryChange := fieldEvent(t, got, "industry")
	if nameChange.Payload["before"] != nil || nameChange.Payload["after"] != "Acme" {
		t.Fatalf("name change %#v", nameChange.Payload)
	}
	if _, ok := employeesChange.Payload["before"]; !ok || employeesChange.Payload["before"] != nil || employeesChange.Payload["after"] != float64(10) {
		t.Fatalf("employees change %#v", employeesChange.Payload)
	}
	if industryChange.Payload["before"] != nil || industryChange.Payload["after"] != "SaaS" {
		t.Fatalf("industry change %#v", industryChange.Payload)
	}

	rec = apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": 1,
		"data":    map[string]any{"industry": "Finance"},
	})
	requireStatus(t, rec, http.StatusOK)
	got = eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	if namesOf(got) != "record.created,field.changed,field.changed,field.changed,record.updated,field.changed" {
		t.Fatalf("update events %s", namesOf(got))
	}
	updated := got[4]
	if updated.Payload["version"] != float64(2) || updated.Payload["object"] != "company" {
		t.Fatalf("update payload %#v", updated.Payload)
	}
	changed := got[5]
	if changed.Payload["field"] != "industry" || changed.Payload["before"] != "SaaS" || changed.Payload["after"] != "Finance" {
		t.Fatalf("field payload %#v", changed.Payload)
	}

	rec = apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": 2,
		"data":    map[string]any{"industry": "Finance"},
	})
	requireStatus(t, rec, http.StatusOK)
	got = eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	if namesOf(got) != "record.created,field.changed,field.changed,field.changed,record.updated,field.changed,record.updated" {
		t.Fatalf("unchanged patch events %s", namesOf(got))
	}
	if len(eventsNamed(got, "field.changed")) != 4 {
		t.Fatalf("field events %s", namesOf(got))
	}

	rec = apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": 1,
		"data":    map[string]any{"name": "Other"},
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "stale version" {
		t.Fatalf("body %s", rec.Body.String())
	}
	if namesOf(eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)) != namesOf(got) {
		t.Fatal("stale patch stored an event")
	}
	if getRecord(t, token, "company", acme.ID).Data["name"] != "Acme" {
		t.Fatal("stale patch changed the record")
	}

	rec = apiDo(t, http.MethodDelete, "/api/objects/company/"+acme.ID, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+acme.ID, token, nil)
	requireStatus(t, rec, http.StatusNotFound)
	got = eventsFor(listEvents(t, token, "record="+acme.ID), acme.ID)
	if got[len(got)-1].Name != "record.deleted" || got[len(got)-1].Payload["object"] != "company" {
		t.Fatalf("delete events %s", namesOf(got))
	}
	if len(eventsNamed(listEvents(t, token, "name=record.deleted"), "record.deleted")) != 1 {
		t.Fatal("record.deleted missing")
	}
	if len(eventsNamed(got, "record.created")) != 1 {
		t.Fatal("delete dropped earlier events")
	}

	all := listEvents(t, token, "")
	var prev int64
	for _, ev := range all {
		if ev.Seq <= prev {
			t.Fatalf("seq %d after %d", ev.Seq, prev)
		}
		prev = ev.Seq
	}
	if len(listEvents(t, token, fmt.Sprintf("after=%d", prev))) != 0 {
		t.Fatal("after the last event returned rows")
	}
	if namesOf(listEvents(t, token, "limit=1")) != "user.created" {
		t.Fatal("limit did not return the oldest event")
	}
	if len(listEvents(t, token, "name=workflow.started")) != 0 {
		t.Fatal("unexpected workflow event")
	}

	other := newOwner(t, "Events Other", "events-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/events?record="+acme.ID, other, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), acme.ID) || strings.Contains(rec.Body.String(), "Acme") {
		t.Fatalf("other org saw %s", rec.Body.String())
	}
	if len(eventsFor(listEvents(t, other, ""), acme.ID)) != 0 {
		t.Fatal("other org listed the record")
	}

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgOf(t, token), "Mia", "events-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodGet, "/api/events", member, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "Acme") || strings.Contains(rec.Body.String(), acme.ID) {
		t.Fatalf("member saw record data %s", rec.Body.String())
	}
	if len(eventsNamed(listEvents(t, member, "name=object.created"), "object.created")) != 1 {
		t.Fatal("member could not see object.created")
	}
	if len(listEvents(t, member, "name=record.created")) != 0 || len(listEvents(t, member, "name=field.changed")) != 0 {
		t.Fatal("member saw record events without a grant")
	}
	memberUsers := eventsNamed(listEvents(t, token, "name=user.created"), "user.created")
	if len(memberUsers) != 2 || memberUsers[1].ActorID != ownerID || memberUsers[1].Payload["email"] != "events-member@example.com" {
		t.Fatalf("member create events %+v %+v", memberUsers, memberUsers)
	}

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", token, map[string]any{
		"role": "member", "can_read": true, "can_write": false,
	})
	requireStatus(t, rec, http.StatusOK)
	if len(eventsNamed(listEvents(t, member, "record="+acme.ID), "record.created")) != 1 {
		t.Fatal("member with read could not see record.created")
	}

	rec = apiDo(t, http.MethodGet, "/api/events", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	if errorOf(t, rec) != "unauthorized" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/events?name=nope", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown event" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/events?record=not-a-uuid", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid record" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/events?limit=0", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid limit" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/events?after=-1", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid after" {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestEventFailureRollsBackRecord(t *testing.T) {
	token := newOwner(t, "EventRollback", "event-rollback@example.com")
	object := postObject(t, token, companyObject())
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id := uuid.NewString()
	orgID := orgOf(t, token)
	if _, err := tx.Exec(ctx, `
		insert into records (id, organization_id, object_id, data)
		values ($1, $2, $3, '{"name":"Ghost"}'::jsonb)
	`, id, orgID, object.ID); err != nil {
		t.Fatal(err)
	}
	_, err = insertEvent(ctx, tx, orgID, eventWrite{
		Name:     "not.an.event",
		ActorID:  userOf(t, token),
		ObjectID: object.ID,
		RecordID: id,
		Source:   sourceAPI,
		Payload:  map[string]any{"object": "company"},
	})
	if err == nil {
		t.Fatal("invalid event name was stored")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var records, events int
	if err := testPool.QueryRow(ctx, `select count(*) from records where id = $1`, id).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `select count(*) from events where record_id = $1`, id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if records != 0 || events != 0 {
		t.Fatalf("rolled back record %d events %d", records, events)
	}
}

func listEvents(t *testing.T, token, query string) []eventView {
	t.Helper()
	path := "/api/events"
	if query != "" {
		path += "?" + query
	}
	rec := apiDo(t, http.MethodGet, path, token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Events []eventView `json:"events"`
	}
	mustJSON(t, rec, &body)
	if body.Events == nil {
		t.Fatal("null events")
	}
	return body.Events
}

func eventsNamed(events []eventView, name string) []eventView {
	out := []eventView{}
	for _, ev := range events {
		if ev.Name == name {
			out = append(out, ev)
		}
	}
	return out
}

func eventsFor(events []eventView, recordID string) []eventView {
	out := []eventView{}
	for _, ev := range events {
		if ev.RecordID != nil && *ev.RecordID == recordID {
			out = append(out, ev)
		}
	}
	return out
}

func namesOf(events []eventView) string {
	names := make([]string, len(events))
	for i, ev := range events {
		names[i] = ev.Name
	}
	return strings.Join(names, ",")
}

func fieldEvent(t *testing.T, events []eventView, field string) eventView {
	t.Helper()
	for _, ev := range events {
		if ev.Name == "field.changed" && ev.Payload["field"] == field {
			return ev
		}
	}
	t.Fatalf("missing field.changed %s", field)
	return eventView{}
}

func userOf(t *testing.T, token string) string {
	t.Helper()
	var id string
	err := testPool.QueryRow(context.Background(), `
		select user_id::text from access_tokens where token_hash = $1
	`, tokenHash(token)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
