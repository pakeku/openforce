package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFiltersSortAndCatalog(t *testing.T) {
	token := newOwner(t, "Catalog", "catalog@example.com")
	postObject(t, token, companyObject())

	rec := apiDo(t, http.MethodGet, "/api/meta/objects/company/views", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var views struct {
		Views []storedView `json:"views"`
	}
	mustJSON(t, rec, &views)
	if len(views.Views) != 1 || views.Views[0].Name != "all" || !views.Views[0].All {
		t.Fatalf("default view %#v", views.Views)
	}
	if len(views.Views[0].Resolved) != 3 {
		t.Fatalf("resolved %#v", views.Views[0].Resolved)
	}

	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/layouts/record", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var layout storedLayout
	mustJSON(t, rec, &layout)
	if layout.Name != "record" || len(layout.Resolved) != 3 || layout.Version != 1 {
		t.Fatalf("layout %#v", layout)
	}

	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/pages", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var pages struct {
		Pages []storedPage `json:"pages"`
	}
	mustJSON(t, rec, &pages)
	if len(pages.Pages) != 2 || pages.Pages[0].Document["kind"] == "" {
		t.Fatalf("pages %#v", pages.Pages)
	}

	postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	postRecord(t, token, "company", map[string]any{"name": "Beta", "employees": 2, "industry": "Finance"})

	saas := listRecords(t, token, "company", "filter=industry:eq:SaaS")
	if len(saas) != 1 || saas[0].Data["name"] != "Acme" {
		t.Fatalf("eq %#v", saas)
	}
	big := listRecords(t, token, "company", "filter=employees:gte:10")
	if len(big) != 1 || big[0].Data["name"] != "Acme" {
		t.Fatalf("gte %#v", big)
	}
	matched := listRecords(t, token, "company", "filter=name:contains:ac")
	if len(matched) != 1 || matched[0].Data["name"] != "Acme" {
		t.Fatalf("contains %#v", matched)
	}
	sorted := listRecords(t, token, "company", "sort=employees:desc")
	if len(sorted) != 2 || sorted[0].Data["name"] != "Acme" || sorted[1].Data["name"] != "Beta" {
		t.Fatalf("sort %#v", sorted)
	}

	rec = apiDo(t, http.MethodGet, "/api/objects/company?filter=website:eq:x", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "unknown field website" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/objects/company?filter=name:before:a", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid operator for name" {
		t.Fatalf("body %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/views", token, map[string]any{
		"name":    "saas",
		"label":   "SaaS",
		"columns": []string{"name"},
		"filters": []map[string]any{{"field": "industry", "operator": "eq", "value": "SaaS"}},
		"sort":    []map[string]any{{"field": "name", "direction": "asc"}},
	})
	requireStatus(t, rec, http.StatusCreated)
	filtered := listRecords(t, token, "company", "view=saas")
	if len(filtered) != 1 || filtered[0].Data["name"] != "Acme" {
		t.Fatalf("view %#v", filtered)
	}
	if _, ok := filtered[0].Data["industry"]; ok {
		t.Fatalf("view kept a hidden column %#v", filtered[0].Data)
	}
	both := listRecords(t, token, "company", "view=saas&filter=name:eq:Beta")
	if len(both) != 0 {
		t.Fatalf("view plus filter %#v", both)
	}

	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/views/saas", token, map[string]any{
		"version": 1,
		"label":   "SaaS companies",
	})
	requireStatus(t, rec, http.StatusOK)
	var updated storedView
	mustJSON(t, rec, &updated)
	if updated.Version != 2 || updated.Label != "SaaS companies" {
		t.Fatalf("patched view %#v", updated)
	}
	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/views/saas", token, map[string]any{
		"version": 1,
		"label":   "stale",
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "stale version" {
		t.Fatalf("body %s", rec.Body.String())
	}

	other := newOwner(t, "Catalog Other", "catalog-other@example.com")
	postObject(t, other, companyObject())
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/views/saas", other, nil)
	requireStatus(t, rec, http.StatusNotFound)

	postObject(t, token, map[string]any{
		"name": "contact", "label": "Contact",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/relationships", token, map[string]any{
		"name": "owner", "kind": "belongs_to", "to": "contact", "inverse": "companies",
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/relationships", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var rels struct {
		Relationships []map[string]string `json:"relationships"`
	}
	mustJSON(t, rec, &rels)
	if len(rels.Relationships) != 1 || rels.Relationships[0]["name"] != "owner" || rels.Relationships[0]["to"] != "contact" {
		t.Fatalf("relationships %#v", rels.Relationships)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/contact/relationships", token, nil)
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &rels)
	if len(rels.Relationships) != 1 || rels.Relationships[0]["name"] != "companies" || rels.Relationships[0]["to"] != "company" {
		t.Fatalf("inverse %#v", rels.Relationships)
	}

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgOf(t, token), "Mia", "catalog-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/views", member, map[string]any{
		"name": "mine", "label": "Mine",
	})
	requireStatus(t, rec, http.StatusForbidden)

	req := httptest.NewRequest(http.MethodOptions, "/api/objects/company", nil)
	opt := httptest.NewRecorder()
	newMux(testPool).ServeHTTP(opt, req)
	requireStatus(t, opt, http.StatusNoContent)
	if opt.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("cors %s", opt.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCatalogReadRequiresGrant(t *testing.T) {
	owner := newOwner(t, "Catalog Read", "catalog-read-owner@example.com")
	postObject(t, owner, companyObject())
	postObject(t, owner, map[string]any{
		"name": "contact", "label": "Contact",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	rec := apiDo(t, http.MethodPost, "/api/meta/objects/company/relationships", owner, map[string]any{
		"name": "owner", "kind": "belongs_to", "to": "contact", "inverse": "companies",
	})
	requireStatus(t, rec, http.StatusCreated)
	if listObjectNames(t, owner) != "company,contact" {
		t.Fatalf("owner list %s", listObjectNames(t, owner))
	}
	for _, path := range companyCatalogReads() {
		rec = apiDo(t, http.MethodGet, path, owner, nil)
		requireStatus(t, rec, http.StatusOK)
	}

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgOf(t, owner), "Mia", "catalog-read-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", member, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.TrimSpace(rec.Body.String()) != `{"objects":[]}` {
		t.Fatalf("member list %s", rec.Body.String())
	}
	for _, path := range companyCatalogReads() {
		requireExactForbidden(t, apiDo(t, http.MethodGet, path, member, nil))
	}
	for _, path := range []string{
		"/api/meta/objects/missing",
		"/api/meta/objects/missing/views",
		"/api/meta/objects/company/views/missing",
	} {
		rec = apiDo(t, http.MethodGet, path, member, nil)
		if path == "/api/meta/objects/company/views/missing" {
			requireExactForbidden(t, rec)
			continue
		}
		requireStatus(t, rec, http.StatusNotFound)
		if errorOf(t, rec) != "not found" {
			t.Fatalf("%s %s", path, rec.Body.String())
		}
	}

	other := newOwner(t, "Catalog Read Other", "catalog-read-other@example.com")
	postObject(t, other, map[string]any{
		"name": "secret", "label": "Secret",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	for _, path := range []string{"/api/meta/objects/secret", "/api/meta/objects/secret/views", "/api/meta/objects/secret/layouts/record", "/api/meta/objects/secret/pages/list", "/api/meta/objects/secret/relationships"} {
		rec = apiDo(t, http.MethodGet, path, member, nil)
		requireStatus(t, rec, http.StatusNotFound)
		if errorOf(t, rec) != "not found" || strings.Contains(rec.Body.String(), "Secret") {
			t.Fatalf("%s %s", path, rec.Body.String())
		}
	}
	if listObjectNames(t, other) != "secret" {
		t.Fatalf("other list %s", listObjectNames(t, other))
	}

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", owner, map[string]any{
		"role": "member", "can_read": false, "can_write": true,
	})
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", member, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.TrimSpace(rec.Body.String()) != `{"objects":[]}` {
		t.Fatalf("write-only list %s", rec.Body.String())
	}
	requireExactForbidden(t, apiDo(t, http.MethodGet, "/api/meta/objects/company", member, nil))

	rec = apiDo(t, http.MethodPut, "/api/meta/objects/company/permissions", owner, map[string]any{
		"role": "member", "can_read": true, "can_write": true,
	})
	requireStatus(t, rec, http.StatusOK)
	if listObjectNames(t, member) != "company" || listObjectNames(t, owner) != "company,contact" {
		t.Fatalf("member %s owner %s", listObjectNames(t, member), listObjectNames(t, owner))
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var object objectView
	mustJSON(t, rec, &object)
	if object.Name != "company" || object.Label != "Company" || findField(t, object.Fields, "name").Type != "text" {
		t.Fatalf("object %#v", object)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/views/all", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var view storedView
	mustJSON(t, rec, &view)
	if view.Name != "all" || !view.All {
		t.Fatalf("view %#v", view)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/layouts/record", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var layout storedLayout
	mustJSON(t, rec, &layout)
	if layout.Name != "record" {
		t.Fatalf("layout %#v", layout)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/pages/list", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var page storedPage
	mustJSON(t, rec, &page)
	if page.Name != "list" {
		t.Fatalf("page %#v", page)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/relationships", member, nil)
	requireStatus(t, rec, http.StatusOK)
	var rels struct {
		Relationships []map[string]string `json:"relationships"`
	}
	mustJSON(t, rec, &rels)
	if len(rels.Relationships) != 1 || rels.Relationships[0]["name"] != "owner" {
		t.Fatalf("relationships %#v", rels.Relationships)
	}
	for _, path := range []string{
		"/api/meta/objects/contact",
		"/api/meta/objects/contact/views",
		"/api/meta/objects/contact/views/all",
		"/api/meta/objects/contact/layouts",
		"/api/meta/objects/contact/layouts/record",
		"/api/meta/objects/contact/pages",
		"/api/meta/objects/contact/pages/record",
		"/api/meta/objects/contact/relationships",
	} {
		requireExactForbidden(t, apiDo(t, http.MethodGet, path, member, nil))
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/views/missing", member, nil)
	requireStatus(t, rec, http.StatusNotFound)
	for _, call := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/meta/objects", map[string]any{"name": "deal", "label": "Deal"}},
		{http.MethodPost, "/api/meta/objects/company/views", map[string]any{"name": "mine", "label": "Mine"}},
		{http.MethodPatch, "/api/meta/objects/company/views/all", map[string]any{"version": 1, "label": "Nope"}},
		{http.MethodPost, "/api/meta/objects/company/layouts", map[string]any{"name": "mine", "label": "Mine"}},
		{http.MethodPatch, "/api/meta/objects/company/layouts/record", map[string]any{"version": 1, "label": "Nope"}},
		{http.MethodPost, "/api/meta/objects/company/pages", map[string]any{"name": "mine", "label": "Mine", "document": map[string]any{"kind": "list", "view": "all"}}},
		{http.MethodPatch, "/api/meta/objects/company/pages/list", map[string]any{"version": 1, "label": "Nope"}},
		{http.MethodPost, "/api/meta/objects/company/relationships", map[string]any{"name": "notes", "kind": "has_many", "to": "contact", "inverse": "company"}},
	} {
		requireExactForbidden(t, apiDo(t, call.method, call.path, member, call.body))
	}

	rec = apiDo(t, http.MethodPost, "/api/keys", owner, map[string]any{
		"name": "reader", "label": "Reader",
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": true}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var key apiKeyView
	mustJSON(t, rec, &key)
	requireExactForbidden(t, apiDo(t, http.MethodGet, "/api/meta/objects", key.Token, nil))
	for _, path := range companyCatalogReads() {
		requireExactForbidden(t, apiDo(t, http.MethodGet, path, key.Token, nil))
	}
}

func companyCatalogReads() []string {
	return []string{
		"/api/meta/objects/company",
		"/api/meta/objects/company/views",
		"/api/meta/objects/company/views/all",
		"/api/meta/objects/company/layouts",
		"/api/meta/objects/company/layouts/record",
		"/api/meta/objects/company/pages",
		"/api/meta/objects/company/pages/list",
		"/api/meta/objects/company/relationships",
	}
}

func listObjectNames(t *testing.T, token string) string {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/meta/objects", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var listed struct {
		Objects []objectView `json:"objects"`
	}
	mustJSON(t, rec, &listed)
	names := make([]string, 0, len(listed.Objects))
	for _, object := range listed.Objects {
		names = append(names, object.Name)
	}
	return strings.Join(names, ",")
}

func requireExactForbidden(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestKanbanView(t *testing.T) {
	token := newOwner(t, "Kanban", "kanban@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `delete from jobs where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())

	rec := apiDo(t, http.MethodGet, "/api/meta/objects/company/views/all", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var all storedView
	mustJSON(t, rec, &all)
	if all.Kind != "table" || all.Group != "" {
		t.Fatalf("default view %#v", all)
	}

	rejected := []struct {
		body map[string]any
		err  string
	}{
		{map[string]any{"name": "pipeline", "label": "Pipeline", "kind": "calendar", "group": "industry"}, "invalid view"},
		{map[string]any{"name": "pipeline", "label": "Pipeline", "kind": "table", "group": "industry"}, "group is only valid for kanban"},
		{map[string]any{"name": "pipeline", "label": "Pipeline", "kind": "kanban", "group": "name"}, "group must be an enum"},
		{map[string]any{"name": "pipeline", "label": "Pipeline", "kind": "kanban"}, "group must be an enum"},
		{map[string]any{"name": "pipeline", "label": "Pipeline", "kind": "kanban", "group": "missing"}, "group must be an enum"},
	}
	for _, item := range rejected {
		rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/views", token, item.body)
		requireStatus(t, rec, http.StatusBadRequest)
		if errorOf(t, rec) != item.err {
			t.Fatalf("body %s", rec.Body.String())
		}
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/views", token, map[string]any{
		"name": "plain", "label": "Plain",
	})
	requireStatus(t, rec, http.StatusCreated)
	var plain storedView
	mustJSON(t, rec, &plain)
	if plain.Kind != "table" || plain.Group != "" {
		t.Fatalf("defaulted kind %#v", plain)
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/views", token, map[string]any{
		"name":    "pipeline",
		"label":   "Pipeline",
		"kind":    "kanban",
		"group":   "industry",
		"columns": []string{"name"},
	})
	requireStatus(t, rec, http.StatusCreated)
	var board storedView
	mustJSON(t, rec, &board)
	if board.Kind != "kanban" || board.Group != "industry" || len(board.Columns) != 1 || board.Columns[0] != "name" {
		t.Fatalf("board %#v", board)
	}

	acme := postRecord(t, token, "company", map[string]any{"name": "Acme", "employees": 10, "industry": "SaaS"})
	postRecord(t, token, "company", map[string]any{"name": "Beta", "employees": 2, "industry": "Finance"})
	listed := listRecords(t, token, "company", "view=pipeline&limit=200")
	var found recordView
	for _, record := range listed {
		if record.Data["name"] == "Acme" {
			found = record
		}
		if _, ok := record.Data["employees"]; ok {
			t.Fatalf("hidden column %#v", record.Data)
		}
	}
	if found.Data["industry"] != "SaaS" {
		t.Fatalf("group column %#v", listed)
	}

	rec = apiDo(t, http.MethodPatch, "/api/objects/company/"+acme.ID, token, map[string]any{
		"version": acme.Version,
		"data":    map[string]any{"industry": "Finance"},
	})
	requireStatus(t, rec, http.StatusOK)
	moved := getRecord(t, token, "company", acme.ID)
	if moved.Data["industry"] != "Finance" || moved.Version != acme.Version+1 {
		t.Fatalf("moved %#v", moved)
	}

	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/views/pipeline", token, map[string]any{
		"version": board.Version,
		"label":   "Industries",
	})
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &board)
	if board.Version != 2 || board.Label != "Industries" || board.Kind != "kanban" || board.Group != "industry" {
		t.Fatalf("label patch %#v", board)
	}

	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/views/pipeline", token, map[string]any{
		"version": board.Version,
		"kind":    "table",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "group is only valid for kanban" {
		t.Fatalf("body %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/fields", token, map[string]any{
		"name": "stage", "type": "enum", "options": []string{"Open", "Won"},
	})
	requireStatus(t, rec, http.StatusCreated)
	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/views/pipeline", token, map[string]any{
		"version": board.Version,
		"group":   "stage",
	})
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &board)
	if board.Kind != "kanban" || board.Group != "stage" || board.Version != 3 {
		t.Fatalf("regroup %#v", board)
	}

	rec = apiDo(t, http.MethodPatch, "/api/meta/objects/company/views/pipeline", token, map[string]any{
		"version": board.Version,
		"kind":    "table",
		"group":   "",
	})
	requireStatus(t, rec, http.StatusOK)
	var cleared storedView
	mustJSON(t, rec, &cleared)
	if cleared.Kind != "table" || cleared.Group != "" {
		t.Fatalf("back to table %#v", cleared)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects/company/views/pipeline", token, nil)
	requireStatus(t, rec, http.StatusOK)
	mustJSON(t, rec, &cleared)
	if cleared.Kind != "table" || cleared.Group != "" {
		t.Fatalf("stored table %#v", cleared)
	}

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "kanban-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/views", member, map[string]any{
		"name": "memberboard", "label": "Member", "kind": "kanban", "group": "industry",
	})
	requireExactForbidden(t, rec)

	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "boardkey", "label": "Board key",
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": true}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var key apiKeyView
	mustJSON(t, rec, &key)
	rec = apiDo(t, http.MethodPost, "/api/meta/objects/company/views", key.Token, map[string]any{
		"name": "keyboard", "label": "Key", "kind": "kanban", "group": "industry",
	})
	requireExactForbidden(t, rec)
}
