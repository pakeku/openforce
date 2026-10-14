package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
