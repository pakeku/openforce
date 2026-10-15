package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestFieldLinkOnCreateAndAdd(t *testing.T) {
	token := newOwner(t, "Field links", "field-links@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `delete from jobs where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())
	acme := postRecord(t, token, "company", map[string]any{
		"name": "Acme", "employees": 10, "industry": "SaaS",
	})

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "field-links-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec := apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "fieldkey", "label": "Field key",
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": true}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var key apiKeyView
	mustJSON(t, rec, &key)
	for _, caller := range []string{member, key.Token} {
		for _, call := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, "/api/meta/objects/company/fields", map[string]any{"name": "note", "type": "text"}},
			{http.MethodPatch, "/api/meta/objects/company/fields/name", map[string]any{"name": "name", "type": "text", "required": true}},
			{http.MethodDelete, "/api/meta/objects/company/fields/name", nil},
		} {
			requireExactForbidden(t, apiDo(t, call.method, call.path, caller, call.body))
		}
	}

	person := postObject(t, token, map[string]any{
		"name":  "person",
		"label": "Person",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
			{"name": "employer", "type": "relation", "to": "company"},
		},
	})
	employer := findField(t, person.Fields, "employer")
	if employer.Type != "relation" || employer.To != "company" || employer.Position != 1 {
		t.Fatalf("employer %#v", employer)
	}
	ada := postRecord(t, token, "person", map[string]any{"name": "Ada", "employer": acme.ID})
	if ada.Data["employer"] != acme.ID || ada.Data["name"] != "Ada" {
		t.Fatalf("person %#v", ada.Data)
	}

	node := postObject(t, token, map[string]any{
		"name":  "node",
		"label": "Node",
		"fields": []map[string]any{
			{"name": "parent", "type": "relation", "to": "node"},
		},
	})
	if findField(t, node.Fields, "parent").To != "node" {
		t.Fatalf("parent %#v", node.Fields)
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects", token, map[string]any{
		"name":  "ghost",
		"label": "Ghost",
		"fields": []map[string]any{
			{"name": "owner", "type": "relation", "to": "missing"},
		},
	})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/ghost", token, nil)
	requireStatus(t, rec, http.StatusNotFound)

	before := getRecord(t, token, "company", acme.ID)
	added := postField(t, token, "company", map[string]any{"name": "note", "type": "text"})
	if added.Name != "note" || added.Type != "text" || added.Position != 3 {
		t.Fatalf("added %#v", added)
	}
	after := getRecord(t, token, "company", acme.ID)
	if after.Version != before.Version || after.Data["name"] != "Acme" || after.Data["note"] != nil {
		t.Fatalf("old record changed %#v", after)
	}
	company := getObject(t, token, "company")
	if findField(t, company.Fields, "note").Type != "text" {
		t.Fatalf("note missing %#v", company.Fields)
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/fields", token, map[string]any{"name": "note", "type": "text"})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "field already exists" {
		t.Fatalf("duplicate %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/fields", token, map[string]any{"name": "calc", "type": "formula"})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "relation and formula are not accepted here" {
		t.Fatalf("formula %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/fields", token, map[string]any{"name": "boss", "type": "relation"})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "relation and formula are not accepted here" {
		t.Fatalf("relation %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/fields", token, map[string]any{
		"name": "boss", "type": "relation", "to": "missing",
	})
	requireStatus(t, rec, http.StatusNotFound)
	company = getObject(t, token, "company")
	for _, field := range company.Fields {
		if field.Name == "boss" {
			t.Fatal("unknown target still added a field")
		}
	}

	linked := postField(t, token, "company", map[string]any{"name": "owner", "type": "relation", "to": "person"})
	if linked.Type != "relation" || linked.To != "person" || linked.Position != 4 {
		t.Fatalf("owner %#v", linked)
	}
	company = getObject(t, token, "company")
	if findField(t, company.Fields, "owner").To != "person" {
		t.Fatalf("loaded %#v", company.Fields)
	}
	owned := patchRecord(t, token, "company", acme.ID, after.Version, map[string]any{"owner": ada.ID})
	if owned.Data["owner"] != ada.ID || owned.Data["name"] != "Acme" {
		t.Fatalf("linked record %#v", owned.Data)
	}
}

func TestFieldEditValuePolicy(t *testing.T) {
	token := newOwner(t, "Field edits", "field-edits@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `delete from jobs where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name":  "person",
		"label": "Person",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})
	acme := postRecord(t, token, "company", map[string]any{
		"name": "Acme", "employees": 10, "industry": "SaaS",
	})
	bare := postRecord(t, token, "company", map[string]any{"name": "Bare"})
	postField(t, token, "company", map[string]any{"name": "note", "type": "text"})
	acme = patchRecord(t, token, "company", acme.ID, acme.Version, map[string]any{"note": "hello"})
	gone := postRecord(t, token, "company", map[string]any{"name": "Gone", "note": "bye"})
	rec := apiDo(t, http.MethodDelete, "/api/objects/company/"+gone.ID, token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	updates := eventCount(t, orgID, "record.updated")
	jobs := jobCount(t, orgID)

	renamed := patchField(t, token, "company", "note", map[string]any{"name": "memo", "type": "text"})
	if renamed.Name != "memo" || renamed.Position != 3 {
		t.Fatalf("renamed %#v", renamed)
	}
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Version != 3 || acme.Data["memo"] != "hello" || acme.Data["note"] != nil || acme.Data["name"] != "Acme" || acme.Data["industry"] != "SaaS" {
		t.Fatalf("renamed record %#v", acme)
	}
	bare = getRecord(t, token, "company", bare.ID)
	if bare.Version != 1 || bare.Data["memo"] != nil || bare.Data["name"] != "Bare" {
		t.Fatalf("untouched %#v", bare)
	}
	version, deleted, data := storedRecord(t, gone.ID)
	if version != 2 || !deleted || data["memo"] != "bye" || data["note"] != nil || data["name"] != "Gone" {
		t.Fatalf("deleted row version %d deleted %v data %#v", version, deleted, data)
	}
	if eventCount(t, orgID, "record.updated") != updates || jobCount(t, orgID) != jobs {
		t.Fatal("rename wrote a record event")
	}

	kept := patchField(t, token, "company", "memo", map[string]any{"name": "memo", "type": "text", "required": true})
	if !kept.Required {
		t.Fatalf("required %#v", kept)
	}
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Version != 3 || acme.Data["memo"] != "hello" {
		t.Fatalf("required cleared %#v", acme)
	}
	patched := patchField(t, token, "company", "industry", map[string]any{
		"name": "industry", "type": "enum", "options": []string{"SaaS", "Finance", "Health"},
	})
	if len(patched.Options) != 3 {
		t.Fatalf("options %#v", patched)
	}
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Version != 3 || acme.Data["industry"] != "SaaS" {
		t.Fatalf("options cleared %#v", acme)
	}

	patchField(t, token, "company", "employees", map[string]any{"name": "employees", "type": "text"})
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Version != 4 || acme.Data["employees"] != nil || acme.Data["name"] != "Acme" || acme.Data["memo"] != "hello" {
		t.Fatalf("type change %#v", acme)
	}
	bare = getRecord(t, token, "company", bare.ID)
	if bare.Version != 1 {
		t.Fatalf("type change touched a row without the field %#v", bare)
	}
	if eventCount(t, orgID, "record.updated") != updates || jobCount(t, orgID) != jobs {
		t.Fatal("type change wrote a record event")
	}

	postField(t, token, "company", map[string]any{"name": "owner", "type": "relation", "to": "person"})
	ada := postRecord(t, token, "person", map[string]any{"name": "Ada"})
	acme = patchRecord(t, token, "company", acme.ID, acme.Version, map[string]any{"owner": ada.ID})
	updates = eventCount(t, orgID, "record.updated")
	jobs = jobCount(t, orgID)
	moved := patchField(t, token, "company", "owner", map[string]any{"name": "owner", "type": "relation", "to": "company"})
	if moved.To != "company" {
		t.Fatalf("retarget %#v", moved)
	}
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Data["owner"] != nil || acme.Data["name"] != "Acme" || acme.Version != 6 {
		t.Fatalf("retarget values %#v", acme)
	}
	if eventCount(t, orgID, "record.updated") != updates || jobCount(t, orgID) != jobs {
		t.Fatal("retarget wrote a record event")
	}

	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/fields/memo", token, map[string]any{
		"name": "name", "type": "text", "required": true,
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "field already exists" {
		t.Fatalf("rename conflict %s", rec.Body.String())
	}
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Data["memo"] != "hello" || acme.Data["name"] != "Acme" {
		t.Fatalf("failed rename %#v", acme.Data)
	}

	rec = apiDo(t, http.MethodDelete, "/api/meta/objects/company/fields/memo", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Data["memo"] != nil || acme.Data["name"] != "Acme" || acme.Version != 7 {
		t.Fatalf("deleted field %#v", acme)
	}
	bare = getRecord(t, token, "company", bare.ID)
	if bare.Version != 1 || bare.Data["name"] != "Bare" {
		t.Fatalf("delete touched another row %#v", bare)
	}
	if eventCount(t, orgID, "record.updated") != updates || jobCount(t, orgID) != jobs {
		t.Fatal("delete wrote a record event")
	}

	postObject(t, token, map[string]any{
		"name":  "solo",
		"label": "Solo",
		"fields": []map[string]any{
			{"name": "only", "type": "text"},
		},
	})
	one := postRecord(t, token, "solo", map[string]any{"only": "Yes"})
	rec = apiDo(t, http.MethodDelete, "/api/meta/objects/solo/fields/only", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	one = getRecord(t, token, "solo", one.ID)
	if one.Data["only"] != nil || one.Version != 2 {
		t.Fatalf("last field %#v", one)
	}
	if len(getObject(t, token, "solo").Fields) != 0 {
		t.Fatal("last field definition stayed")
	}

	postObject(t, token, map[string]any{
		"name":  "comment",
		"label": "Comment",
		"fields": []map[string]any{
			{"name": "body", "type": "text"},
		},
	})
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/comment/relationships", token, map[string]any{
		"name": "task", "kind": "belongs_to", "to": "company", "inverse": "comments",
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/relationships", token, map[string]any{
		"name": "comments", "kind": "has_many", "to": "comment", "inverse": "task",
	})
	requireStatus(t, rec, http.StatusCreated)
	comment := postRecord(t, token, "comment", map[string]any{"body": "Hi", "task": acme.ID})
	patchField(t, token, "comment", "task", map[string]any{"name": "account", "type": "relation", "to": "company"})
	comment = getRecord(t, token, "comment", comment.ID)
	if comment.Data["account"] != acme.ID || comment.Data["task"] != nil || comment.Data["body"] != "Hi" || comment.Version != 2 {
		t.Fatalf("renamed link %#v", comment)
	}
	list := findRel(t, token, "company", "comments", "has_many")
	if list["inverse"] != "account" || list["to"] != "comment" {
		t.Fatalf("child list %#v", list)
	}

	postObject(t, token, map[string]any{
		"name":  "reply",
		"label": "Reply",
		"fields": []map[string]any{
			{"name": "body", "type": "text"},
			{"name": "ticket", "type": "relation", "to": "company"},
		},
	})
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/relationships", token, map[string]any{
		"name": "replies", "kind": "has_many", "to": "reply", "inverse": "ticket",
	})
	requireStatus(t, rec, http.StatusCreated)
	reply := postRecord(t, token, "reply", map[string]any{"body": "Ok", "ticket": acme.ID})
	patchField(t, token, "reply", "ticket", map[string]any{"name": "ticket", "type": "relation", "to": "person"})
	reply = getRecord(t, token, "reply", reply.ID)
	if reply.Data["ticket"] != nil || reply.Data["body"] != "Ok" || reply.Version != 2 {
		t.Fatalf("retarget link %#v", reply)
	}
	if findField(t, getObject(t, token, "reply").Fields, "ticket").To != "person" {
		t.Fatal("ticket target was not updated")
	}
	if rel, ok := maybeRel(t, token, "company", "replies", "has_many"); ok {
		t.Fatalf("child list stayed %#v", rel)
	}

	rec = apiDo(t, http.MethodDelete, "/api/meta/objects/comment/fields/account", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	comment = getRecord(t, token, "comment", comment.ID)
	if comment.Data["account"] != nil || comment.Data["body"] != "Hi" {
		t.Fatalf("deleted link %#v", comment.Data)
	}
	if rel, ok := maybeRel(t, token, "company", "comments", "has_many"); ok {
		t.Fatalf("deleted link left a child list %#v", rel)
	}

	postObject(t, token, map[string]any{
		"name":  "tag",
		"label": "Tag",
		"fields": []map[string]any{
			{"name": "name", "type": "text", "required": true},
		},
	})
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/relationships", token, map[string]any{
		"name": "tags", "kind": "many_to_many", "to": "tag", "inverse": "companies",
	})
	requireStatus(t, rec, http.StatusCreated)
	postField(t, token, "company", map[string]any{"name": "tags", "type": "text"})
	acme = patchRecord(t, token, "company", acme.ID, acme.Version, map[string]any{"tags": "hot"})
	rec = apiDo(t, http.MethodDelete, "/api/meta/objects/company/fields/tags", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	acme = getRecord(t, token, "company", acme.ID)
	if acme.Data["tags"] != nil || acme.Data["name"] != "Acme" {
		t.Fatalf("same-name field %#v", acme.Data)
	}
	list = findRel(t, token, "company", "tags", "many_to_many")
	if list["to"] != "tag" {
		t.Fatalf("links removed %#v", list)
	}

	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/fields/missing", token, map[string]any{"name": "missing", "type": "text"})
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodDelete, "/api/meta/objects/company/fields/missing", token, nil)
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/missing/fields", token, map[string]any{"name": "note", "type": "text"})
	requireStatus(t, rec, http.StatusNotFound)
}

func postField(t *testing.T, token, object string, body map[string]any) fieldView {
	t.Helper()
	rec := apiDo(t, http.MethodPost, "/api/meta/objects/"+object+"/fields", token, body)
	requireStatus(t, rec, http.StatusCreated)
	var field fieldView
	mustJSON(t, rec, &field)
	return field
}

func patchField(t *testing.T, token, object, name string, body map[string]any) fieldView {
	t.Helper()
	rec := apiDo(t, http.MethodPatch, "/api/meta/objects/"+object+"/fields/"+name, token, body)
	requireStatus(t, rec, http.StatusOK)
	var field fieldView
	mustJSON(t, rec, &field)
	return field
}

func patchRecord(t *testing.T, token, object, id string, version int, data map[string]any) recordView {
	t.Helper()
	rec := apiDo(t, http.MethodPatch, "/api/objects/"+object+"/"+id, token, map[string]any{
		"version": version,
		"data":    data,
	})
	requireStatus(t, rec, http.StatusOK)
	var view recordView
	mustJSON(t, rec, &view)
	return view
}

func getObject(t *testing.T, token, name string) objectView {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/meta/objects/"+name, token, nil)
	requireStatus(t, rec, http.StatusOK)
	var view objectView
	mustJSON(t, rec, &view)
	return view
}

func findRel(t *testing.T, token, object, name, kind string) map[string]string {
	t.Helper()
	rel, ok := maybeRel(t, token, object, name, kind)
	if !ok {
		t.Fatalf("missing %s %s on %s", kind, name, object)
	}
	return rel
}

func maybeRel(t *testing.T, token, object, name, kind string) (map[string]string, bool) {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/meta/objects/"+object+"/relationships", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var body struct {
		Relationships []map[string]string `json:"relationships"`
	}
	mustJSON(t, rec, &body)
	for _, item := range body.Relationships {
		if item["name"] == name && item["kind"] == kind {
			return item, true
		}
	}
	return nil, false
}

func eventCount(t *testing.T, orgID, name string) int {
	t.Helper()
	var n int
	err := testPool.QueryRow(context.Background(), `
		select count(*) from events where organization_id = $1::uuid and name = $2
	`, orgID, name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func jobCount(t *testing.T, orgID string) int {
	t.Helper()
	var n int
	err := testPool.QueryRow(context.Background(), `
		select count(*) from jobs where organization_id = $1::uuid
	`, orgID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func storedRecord(t *testing.T, id string) (int, bool, map[string]any) {
	t.Helper()
	var version int
	var deleted bool
	var raw []byte
	err := testPool.QueryRow(context.Background(), `
		select version, deleted_at is not null, data from records where id = $1::uuid
	`, id).Scan(&version, &deleted, &raw)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return version, deleted, data
}
