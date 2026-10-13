package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestHealthAgainstDatabase(t *testing.T) {
	rec := apiDo(t, http.MethodGet, "/healthz", "", nil)
	requireStatus(t, rec, http.StatusOK)
	var body map[string]string
	mustJSON(t, rec, &body)
	if body["status"] != "ok" {
		t.Fatalf("status %#v", body)
	}
}

func TestSeedAndBearerTokens(t *testing.T) {
	if code := runSeed([]string{"-org", "Missing"}); code != 2 {
		t.Fatalf("seed without email and name returned %d", code)
	}

	t.Setenv("DATABASE_URL", testURL)
	org1, token1 := captureSeed(t, "-org", "North", "-email", "north@example.com", "-name", "Nia")
	org2, token2 := captureSeed(t, "-org", "South", "-email", "south@example.com", "-name", "Sam")
	if org1 == org2 || token1 == token2 {
		t.Fatal("second seed did not create its own organization and token")
	}

	rec := apiDo(t, http.MethodGet, "/api/meta/objects", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	if errorOf(t, rec) != "unauthorized" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", "not-a-token", nil)
	requireStatus(t, rec, http.StatusUnauthorized)

	rec = apiDo(t, http.MethodGet, "/api/meta/objects", token1, nil)
	requireStatus(t, rec, http.StatusOK)
	var listed struct {
		Objects []objectView `json:"objects"`
	}
	mustJSON(t, rec, &listed)
	if len(listed.Objects) != 0 {
		t.Fatalf("new org objects %#v", listed.Objects)
	}

	ctx := context.Background()
	var stored string
	if err := testPool.QueryRow(ctx, `select token_hash from access_tokens where organization_id = $1`, org1).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != tokenHash(token1) || stored == token1 {
		t.Fatalf("token stored as %s", stored)
	}

	var teams, members bool
	if err := testPool.QueryRow(ctx, `
		select to_regclass('public.teams') is not null, to_regclass('public.team_members') is not null
	`).Scan(&teams, &members); err != nil {
		t.Fatal(err)
	}
	if !teams || !members {
		t.Fatal("teams tables were not created")
	}

	tag, err := testPool.Exec(ctx, `delete from memberships where organization_id = $1`, org1)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("removed %d memberships", tag.RowsAffected())
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", token1, nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", token2, nil)
	requireStatus(t, rec, http.StatusOK)
}

func TestObjectDefinitions(t *testing.T) {
	token := newOwner(t, "Objects", "objects@example.com")
	created := postObject(t, token, companyObject())

	rec := apiDo(t, http.MethodGet, "/api/meta/objects/company", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var view objectView
	mustJSON(t, rec, &view)
	if view.ID != created.ID || view.Label != "Company" {
		t.Fatalf("object %#v", view)
	}
	name := findField(t, view.Fields, "name")
	if name.Type != "text" || !name.Required || name.Position != 0 {
		t.Fatalf("name field %#v", name)
	}
	employees := findField(t, view.Fields, "employees")
	if employees.Type != "number" || employees.Required || employees.Position != 1 {
		t.Fatalf("employees field %#v", employees)
	}
	industry := findField(t, view.Fields, "industry")
	if industry.Type != "enum" || industry.Position != 2 || strings.Join(industry.Options, ",") != "SaaS,Finance" {
		t.Fatalf("industry field %#v", industry)
	}

	rec = apiDo(t, http.MethodGet, "/api/meta/objects", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var listed struct {
		Objects []objectView `json:"objects"`
	}
	mustJSON(t, rec, &listed)
	if len(listed.Objects) != 1 || listed.Objects[0].ID != created.ID {
		t.Fatalf("list %#v", listed.Objects)
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects", token, companyObject())
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "object already exists" {
		t.Fatalf("body %s", rec.Body.String())
	}

	other := newOwner(t, "Objects Other", "objects-other@example.com")
	again := postObject(t, other, companyObject())
	if again.ID == created.ID {
		t.Fatal("organizations share an object id")
	}

	for _, fieldType := range []string{"relation", "formula"} {
		rec = apiDo(t, http.MethodPost, "/api/meta/objects", token, map[string]any{
			"name":  "deal",
			"label": "Deal",
			"fields": []map[string]any{
				{"name": "parent", "type": fieldType},
			},
		})
		requireStatus(t, rec, http.StatusBadRequest)
		if errorOf(t, rec) != "relation and formula are not accepted here" {
			t.Fatalf("%s body %s", fieldType, rec.Body.String())
		}
	}
}

func TestRecordCRUD(t *testing.T) {
	token := newOwner(t, "Records", "crud@example.com")
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "person",
		"label": "Person",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})

	rejects := []struct {
		body map[string]any
		msg  string
	}{
		{map[string]any{"employees": 1, "industry": "SaaS"}, "missing required field name"},
		{map[string]any{"name": "Acme", "website": "acme.test"}, "unknown field website"},
		{map[string]any{"name": "Acme", "employees": "ten", "industry": "SaaS"}, "invalid value for employees"},
		{map[string]any{"name": "Acme", "employees": 10, "industry": "Nope"}, "invalid value for industry"},
	}
	for _, tc := range rejects {
		rec := apiDo(t, http.MethodPost, "/api/objects/company", token, tc.body)
		requireStatus(t, rec, http.StatusBadRequest)
		if errorOf(t, rec) != tc.msg {
			t.Fatalf("got %s, want %s", rec.Body.String(), tc.msg)
		}
	}

	acme := postRecord(t, token, "company", map[string]any{
		"name": "Acme", "employees": 10, "industry": "SaaS",
	})
	if acme.Version != 1 || acme.Data["name"] != "Acme" || acme.Data["employees"] != float64(10) || acme.Data["industry"] != "SaaS" {
		t.Fatalf("created %#v", acme)
	}
	got := getRecord(t, token, "company", acme.ID)
	if got.ID != acme.ID || got.Version != 1 || got.Data["name"] != "Acme" {
		t.Fatalf("read %#v", got)
	}

	rec := apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": 1,
		"data":    map[string]any{"employees": 12},
	})
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &got)
	if got.Version != 2 || got.Data["employees"] != float64(12) || got.Data["name"] != "Acme" || got.Data["industry"] != "SaaS" {
		t.Fatalf("patched %#v", got)
	}

	umbrella := postRecord(t, token, "company", map[string]any{
		"name": "Umbrella", "employees": 2, "industry": "Finance",
	})
	person := postRecord(t, token, "person", map[string]any{"name": "Pat"})
	listed := listRecords(t, token, "company", "")
	seen := map[string]bool{}
	for _, row := range listed {
		seen[row.ID] = true
		if row.ID == person.ID {
			t.Fatal("person listed with companies")
		}
	}
	if len(listed) != 2 || !seen[acme.ID] || !seen[umbrella.ID] {
		t.Fatalf("list %#v", listed)
	}
	if len(listRecords(t, token, "company", "limit=1")) != 1 {
		t.Fatal("limit did not apply")
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company?limit=0", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid limit" {
		t.Fatalf("body %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodGet, "/api/objects/company/not-a-uuid", token, nil)
	requireStatus(t, rec, http.StatusNotFound)

	rec = apiDo(t, http.MethodDelete, "/api/objects/company/"+acme.ID, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	if rec.Body.Len() != 0 {
		t.Fatalf("delete body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+acme.ID, token, nil)
	requireStatus(t, rec, http.StatusNotFound)
	left := listRecords(t, token, "company", "")
	if len(left) != 1 || left[0].ID != umbrella.ID {
		t.Fatalf("after delete %#v", left)
	}
}

func TestTenantIsolation(t *testing.T) {
	a := newOwner(t, "Org A", "a@example.com")
	b := newOwner(t, "Org B", "b@example.com")
	postObject(t, a, companyObject())
	postObject(t, b, companyObject())
	postObject(t, b, map[string]any{
		"name":  "secret",
		"label": "Secret",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})
	aRec := postRecord(t, a, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	bRec := postRecord(t, b, "company", map[string]any{"name": "Beta", "employees": 3, "industry": "Finance"})

	listed := listRecords(t, a, "company", "")
	if len(listed) != 1 || listed[0].ID != aRec.ID || listed[0].Data["name"] != "Acme" {
		t.Fatalf("A list %#v", listed)
	}

	rec := apiDo(t, http.MethodGet, "/api/meta/objects", a, nil)
	requireStatus(t, rec, http.StatusOK)
	var objects struct {
		Objects []objectView `json:"objects"`
	}
	mustJSON(t, rec, &objects)
	if len(objects.Objects) != 1 || objects.Objects[0].Name != "company" {
		t.Fatalf("A objects %#v", objects.Objects)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/secret", a, nil)
	requireStatus(t, rec, http.StatusNotFound)

	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+bRec.ID, a, nil)
	requireStatus(t, rec, http.StatusNotFound)
	if strings.Contains(rec.Body.String(), "Beta") || errorOf(t, rec) != "not found" {
		t.Fatalf("body %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPatch, "/api/objects/company/"+bRec.ID, a, map[string]any{
		"version": 1,
		"data":    map[string]any{"name": "Stolen"},
	})
	requireStatus(t, rec, http.StatusNotFound)
	if getRecord(t, b, "company", bRec.ID).Data["name"] != "Beta" {
		t.Fatal("A patched B's record")
	}

	rec = apiDo(t, http.MethodDelete, "/api/objects/company/"+bRec.ID, a, nil)
	requireStatus(t, rec, http.StatusNotFound)
	if getRecord(t, b, "company", bRec.ID).ID != bRec.ID {
		t.Fatal("A deleted B's record")
	}
}

func TestRelationships(t *testing.T) {
	token := newOwner(t, "Relations", "rel@example.com")
	postObject(t, token, map[string]any{
		"name": "contact", "label": "Contact",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	postObject(t, token, map[string]any{
		"name": "company", "label": "Company",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	postObject(t, token, map[string]any{
		"name": "tag", "label": "Tag",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})

	rec := apiDo(t, http.MethodPost, "/api/meta/objects/company/relationships", token, map[string]any{
		"name": "owner", "kind": "belongs_to", "to": "contact", "inverse": "companies",
	})
	requireStatus(t, rec, http.StatusCreated)

	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var companyDef objectView
	mustJSON(t, rec, &companyDef)
	owner := findField(t, companyDef.Fields, "owner")
	if owner.Type != "relation" {
		t.Fatalf("owner field %#v", owner)
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/contact/relationships", token, map[string]any{
		"name": "deals", "kind": "has_many", "to": "company", "inverse": "nope",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "inverse belongs_to was not found" {
		t.Fatalf("body %s", rec.Body.String())
	}

	contact := postRecord(t, token, "contact", map[string]any{"name": "Ada"})
	other := newOwner(t, "Relations Other", "rel-other@example.com")
	postObject(t, other, map[string]any{
		"name": "contact", "label": "Contact",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	postObject(t, other, map[string]any{
		"name": "tag", "label": "Tag",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	foreignContact := postRecord(t, other, "contact", map[string]any{"name": "Foreign"})
	foreignTag := postRecord(t, other, "tag", map[string]any{"name": "ForeignTag"})
	wrong := postRecord(t, token, "tag", map[string]any{"name": "Priority"})

	for _, ownerID := range []string{"not-a-uuid", foreignContact.ID, wrong.ID} {
		rec = apiDo(t, http.MethodPost, "/api/objects/company", token, map[string]any{
			"name": "Ghost", "owner": ownerID,
		})
		requireStatus(t, rec, http.StatusBadRequest)
		if errorOf(t, rec) != "invalid value for owner" {
			t.Fatalf("owner %s body %s", ownerID, rec.Body.String())
		}
	}

	company := postRecord(t, token, "company", map[string]any{"name": "Acme", "owner": contact.ID})
	got := getRecord(t, token, "company", company.ID)
	if got.Data["owner"] != contact.ID || got.Data["name"] != "Acme" {
		t.Fatalf("company %#v", got)
	}
	companies := listRelated(t, token, "contact", contact.ID, "companies")
	if !containsID(companies, company.ID) {
		t.Fatalf("inverse companies %#v", companies)
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/contact/relationships", token, map[string]any{
		"name": "employers", "kind": "has_many", "to": "company", "inverse": "owner",
	})
	requireStatus(t, rec, http.StatusCreated)
	employers := listRelated(t, token, "contact", contact.ID, "employers")
	if !containsID(employers, company.ID) {
		t.Fatalf("has_many %#v", employers)
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/relationships", token, map[string]any{
		"name": "tags", "kind": "many_to_many", "to": "tag", "inverse": "companies",
	})
	requireStatus(t, rec, http.StatusCreated)
	tag := postRecord(t, token, "tag", map[string]any{"name": "Customer"})
	rec = apiDo(t, http.MethodPost, "/api/objects/company/"+company.ID+"/related/tags", token, map[string]string{"id": tag.ID})
	requireStatus(t, rec, http.StatusCreated)

	tags := listRelated(t, token, "company", company.ID, "tags")
	if len(tags) != 1 || tags[0].ID != tag.ID || tags[0].Data["name"] != "Customer" {
		t.Fatalf("tags %#v", tags)
	}
	linked := listRelated(t, token, "tag", tag.ID, "companies")
	if !containsID(linked, company.ID) {
		t.Fatalf("tag companies %#v", linked)
	}

	rec = apiDo(t, http.MethodPost, "/api/objects/company/"+company.ID+"/related/tags", token, map[string]string{"id": foreignTag.ID})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPost, "/api/objects/company/"+company.ID+"/related/tags", token, map[string]string{"id": contact.ID})
	requireStatus(t, rec, http.StatusNotFound)
	tags = listRelated(t, token, "company", company.ID, "tags")
	if len(tags) != 1 || tags[0].ID != tag.ID {
		t.Fatalf("tags after rejected links %#v", tags)
	}

	postObject(t, token, map[string]any{
		"name": "note", "label": "Note",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/note/relationships", token, map[string]any{
		"name": "companies", "kind": "has_many", "to": "company", "inverse": "owner",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "inverse belongs_to was not found" {
		t.Fatalf("body %s", rec.Body.String())
	}

	ctx := context.Background()
	var tagObject, relID string
	if err := testPool.QueryRow(ctx, `
		select id::text from object_definitions where organization_id = $1 and name = 'tag'
	`, orgOf(t, token)).Scan(&tagObject); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `
		select id::text from relationship_definitions where organization_id = $1 and name = 'tags'
	`, orgOf(t, token)).Scan(&relID); err != nil {
		t.Fatal(err)
	}
	leakedID := "11111111-1111-1111-1111-111111111111"
	if _, err := testPool.Exec(ctx, `
		insert into records (id, organization_id, object_id, data)
		values ($1, $2, $3, '{"name":"Leaked"}')
	`, leakedID, orgOf(t, other), tagObject); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `
		insert into record_links (id, organization_id, relationship_id, from_record_id, to_record_id)
		values ($1, $2, $3, $4, $5)
	`, "22222222-2222-2222-2222-222222222222", orgOf(t, token), relID, company.ID, leakedID); err != nil {
		t.Fatal(err)
	}
	for _, row := range listRelated(t, token, "company", company.ID, "tags") {
		if row.ID == leakedID || row.Data["name"] == "Leaked" {
			t.Fatal("cross-organization link was visible")
		}
	}

	member, err := (&server{pool: testPool}).addMember(ctx, orgOf(t, token), "Mia", "rel-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", token, map[string]any{
		"role": "member", "can_read": true, "can_write": true,
	})
	requireStatus(t, rec, http.StatusOK)
	follow := postRecord(t, token, "tag", map[string]any{"name": "Followup"})
	rec = apiDo(t, http.MethodPost, "/api/objects/company/"+company.ID+"/related/tags", member, map[string]string{"id": follow.ID})
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodPut, "/api/meta/objects/tag/permissions", token, map[string]any{
		"role": "member", "can_read": true, "can_write": false,
	})
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodPost, "/api/objects/company/"+company.ID+"/related/tags", member, map[string]string{"id": follow.ID})
	requireStatus(t, rec, http.StatusCreated)
}

func TestObjectPolicy(t *testing.T) {
	owner := newOwner(t, "Policy", "policy-owner@example.com")
	postObject(t, owner, companyObject())
	ctx := context.Background()
	var grants int
	if err := testPool.QueryRow(ctx, `
		select count(*) from object_permissions
		where organization_id = (select organization_id from access_tokens where token_hash = $1)
	`, tokenHash(owner)).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("owner stored %d grants", grants)
	}
	acme := postRecord(t, owner, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	if getRecord(t, owner, "company", acme.ID).Data["name"] != "Acme" {
		t.Fatal("owner could not read without a grant")
	}

	member, err := (&server{pool: testPool}).addMember(ctx, orgOf(t, owner), "Mia", "policy-member@example.com")
	if err != nil {
		t.Fatal(err)
	}

	rec := apiDo(t, http.MethodGet, "/api/objects/company", member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if errorOf(t, rec) != "forbidden" || strings.Contains(rec.Body.String(), "Acme") {
		t.Fatalf("list body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company/"+acme.ID, member, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if strings.Contains(rec.Body.String(), "Acme") {
		t.Fatalf("get body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/objects/company", member, map[string]any{
		"name": "Other", "employees": 1, "industry": "SaaS",
	})
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", member, map[string]any{
		"role": "member", "can_read": true, "can_write": true,
	})
	requireStatus(t, rec, http.StatusForbidden)

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", owner, map[string]any{
		"role": "owner", "can_read": true, "can_write": true,
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "only the member role can be granted" {
		t.Fatalf("body %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", owner, map[string]any{
		"role": "member", "can_read": true, "can_write": false,
	})
	requireStatus(t, rec, http.StatusOK)
	listed := listRecords(t, member, "company", "")
	if len(listed) != 1 || listed[0].ID != acme.ID {
		t.Fatalf("member read %#v", listed)
	}
	rec = apiDo(t, http.MethodPost, "/api/objects/company", member, map[string]any{
		"name": "Other", "employees": 1, "industry": "SaaS",
	})
	requireStatus(t, rec, http.StatusForbidden)

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", owner, map[string]any{
		"role": "member", "can_read": true, "can_write": true,
	})
	requireStatus(t, rec, http.StatusOK)
	created := postRecord(t, member, "company", map[string]any{
		"name": "Other", "employees": 1, "industry": "SaaS",
	})
	if created.Data["name"] != "Other" || created.Version != 1 {
		t.Fatalf("member create %#v", created)
	}
}

func TestSoftDeleteAndVersion(t *testing.T) {
	token := newOwner(t, "Versions", "version@example.com")
	postObject(t, token, companyObject())
	created := postRecord(t, token, "company", map[string]any{
		"name": "Acme", "employees": 10, "industry": "SaaS",
	})
	if created.Version != 1 {
		t.Fatalf("version %d", created.Version)
	}

	path := "/api/objects/company/" + created.ID
	payloads := []float64{11, 99}
	results := make([]struct {
		code int
		body string
		view recordView
	}, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range payloads {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			rec, err := apiCall(http.MethodPatch, path, token, map[string]any{
				"version": 1,
				"data":    map[string]any{"employees": payloads[i]},
			})
			if err != nil {
				results[i].code = -1
				results[i].body = err.Error()
				return
			}
			results[i].code = rec.Code
			results[i].body = rec.Body.String()
			if rec.Code == http.StatusOK {
				_ = json.Unmarshal(rec.Body.Bytes(), &results[i].view)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	var wins, conflicts int
	var winner recordView
	for _, res := range results {
		switch res.code {
		case http.StatusOK:
			wins++
			winner = res.view
		case http.StatusConflict:
			conflicts++
			if !strings.Contains(res.body, "stale version") || strings.Contains(res.body, "Acme") {
				t.Fatalf("conflict body %s", res.body)
			}
		default:
			t.Fatalf("patch status %d body %s", res.code, res.body)
		}
	}
	if wins != 1 || conflicts != 1 || winner.Version != 2 {
		t.Fatalf("wins %d conflicts %d winner %#v", wins, conflicts, winner)
	}
	if winner.Data["name"] != "Acme" || winner.Data["industry"] != "SaaS" {
		t.Fatalf("winner dropped fields %#v", winner.Data)
	}
	got := getRecord(t, token, "company", created.ID)
	if got.Version != 2 || got.Data["employees"] != winner.Data["employees"] || got.Data["name"] != "Acme" {
		t.Fatalf("stored %#v, winner %#v", got, winner)
	}

	rec := apiDo(t, http.MethodDelete, path, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodGet, path, token, nil)
	requireStatus(t, rec, http.StatusNotFound)
	for _, row := range listRecords(t, token, "company", "") {
		if row.ID == created.ID {
			t.Fatal("deleted record still listed")
		}
	}
	version, deleted := recordState(t, created.ID)
	if version != 2 || !deleted {
		t.Fatalf("version %d deleted %v", version, deleted)
	}

	rec = apiDo(t, http.MethodPatch, path, token, map[string]any{
		"version": 2,
		"data":    map[string]any{"employees": 5},
	})
	requireStatus(t, rec, http.StatusNotFound)
	version, deleted = recordState(t, created.ID)
	if version != 2 || !deleted {
		t.Fatalf("after patch of deleted: version %d deleted %v", version, deleted)
	}
}

func companyObject() map[string]any {
	return map[string]any{
		"name":  "company",
		"label": "Company",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "employees", "type": "number"},
			{"name": "industry", "type": "enum", "options": []string{"SaaS", "Finance"}},
		},
	}
}

func newOwner(t *testing.T, org, email string) string {
	t.Helper()
	_, token, err := (&server{pool: testPool}).bootstrap(context.Background(), org, "Owner", email, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func orgOf(t *testing.T, token string) string {
	t.Helper()
	var id string
	err := testPool.QueryRow(context.Background(), `
		select organization_id::text from access_tokens where token_hash = $1
	`, tokenHash(token)).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func recordState(t *testing.T, id string) (int, bool) {
	t.Helper()
	var version int
	var deleted bool
	err := testPool.QueryRow(context.Background(), `
		select version, deleted_at is not null from records where id = $1
	`, id).Scan(&version, &deleted)
	if err != nil {
		t.Fatal(err)
	}
	return version, deleted
}

func captureSeed(t *testing.T, args ...string) (string, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := runSeed(args)
	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("seed %v returned %d\n%s", args, code, out)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if ok {
			fields[key] = strings.TrimSpace(value)
		}
	}
	if fields["organization"] == "" || fields["token"] == "" {
		t.Fatalf("seed output %s", out)
	}
	return fields["organization"], fields["token"]
}

func postObject(t *testing.T, token string, body map[string]any) objectView {
	t.Helper()
	rec := apiDo(t, http.MethodPost, "/api/meta/objects", token, body)
	requireStatus(t, rec, http.StatusCreated)
	var view objectView
	mustJSON(t, rec, &view)
	return view
}

func postRecord(t *testing.T, token, name string, data map[string]any) recordView {
	t.Helper()
	rec := apiDo(t, http.MethodPost, "/api/objects/"+name, token, data)
	requireStatus(t, rec, http.StatusCreated)
	var view recordView
	mustJSON(t, rec, &view)
	return view
}

func getRecord(t *testing.T, token, name, id string) recordView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/objects/"+name+"/"+id, token, nil)
	requireStatus(t, rec, http.StatusOK)
	var view recordView
	mustJSON(t, rec, &view)
	return view
}

func listRecords(t *testing.T, token, name, query string) []recordView {
	t.Helper()
	path := "/api/objects/" + name
	if query != "" {
		path += "?" + query
	}
	rec := apiDo(t, http.MethodGet, path, token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Records []recordView `json:"records"`
	}
	mustJSON(t, rec, &body)
	return body.Records
}

func listRelated(t *testing.T, token, object, id, rel string) []recordView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/objects/"+object+"/"+id+"/related/"+rel, token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Records []recordView `json:"records"`
	}
	mustJSON(t, rec, &body)
	return body.Records
}

func containsID(records []recordView, id string) bool {
	for _, rec := range records {
		if rec.ID == id {
			return true
		}
	}
	return false
}

func findField(t *testing.T, fields []fieldView, name string) fieldView {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("missing field %s", name)
	return fieldView{}
}

func apiDo(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec, err := apiCall(method, path, token, body)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func apiCall(method, path, token string, body any) (*httptest.ResponseRecorder, error) {
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	newMux(testPool).ServeHTTP(rec, req)
	return rec, nil
}

func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d, body %s", rec.Code, status, rec.Body.String())
	}
}

func mustJSON(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("status %d body %s: %v", rec.Code, rec.Body.String(), err)
	}
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	mustJSON(t, rec, &body)
	return body.Error
}
