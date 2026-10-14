package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAPIRecordDocument(t *testing.T) {
	token := newOwner(t, "OpenAPI", "openapi-owner@example.com")
	orgID := orgOf(t, token)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `delete from jobs where organization_id = $1::uuid`, orgID)
	})
	postObject(t, token, companyObject())
	postObject(t, token, map[string]any{
		"name": "note", "label": "Note",
		"fields": []map[string]any{
			{"name": "title", "type": "text", "required": true},
			{"name": "amount", "type": "currency"},
			{"name": "active", "type": "boolean"},
			{"name": "opened", "type": "date"},
		},
	})
	rec := apiDo(t, http.MethodPost, "/api/meta/objects/note/relationships", token, map[string]any{
		"name": "account", "kind": "belongs_to", "to": "company",
	})
	requireStatus(t, rec, http.StatusCreated)

	rec = apiDo(t, http.MethodGet, "/api/openapi.json", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	if errorOf(t, rec) != "unauthorized" {
		t.Fatalf("missing token %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodGet, "/api/openapi.json", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("content type %s", rec.Header().Get("Content-Type"))
	}
	doc := decodeOpenAPI(t, rec)
	if doc["openapi"] != "3.0.3" {
		t.Fatalf("openapi %#v", doc["openapi"])
	}
	assertBearer(t, doc)
	paths := pathItems(t, doc)
	if len(paths) != 4 {
		t.Fatalf("paths %#v", pathKeys(paths))
	}
	for key := range paths {
		if !strings.HasPrefix(key, "/api/objects/") || strings.Contains(key, "/audit") || strings.Contains(key, "/related") {
			t.Fatalf("unexpected path %s", key)
		}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"/api/meta", "/api/webhooks", "/api/credentials", "/api/automations", "/api/keys", "/api/connectors", "/api/inbound", "/api/jobs", "/api/events"} {
		if strings.Contains(string(raw), hidden) {
			t.Fatalf("documented %s", hidden)
		}
	}

	company := paths["/api/objects/company"]
	companyItem := paths["/api/objects/company/{id}"]
	if company == nil || companyItem == nil {
		t.Fatalf("company paths %#v", pathKeys(paths))
	}
	assertCompanyFixture(t, doc, company, companyItem)
	assertNoteTypes(t, doc, paths)

	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "openapi-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	assertEmptyPaths(t, member)
	rec = apiDo(t, http.MethodPut, "/api/meta/objects/note/permissions", token, map[string]any{
		"role": "member", "can_read": false, "can_write": true,
	})
	requireStatus(t, rec, http.StatusOK)
	assertEmptyPaths(t, member)
	rec = apiDo(t, http.MethodPut, "/api/meta/objects/note/permissions", token, map[string]any{
		"role": "member", "can_read": true, "can_write": false,
	})
	requireStatus(t, rec, http.StatusOK)
	memberPaths := pathItems(t, openAPIDoc(t, member))
	if len(memberPaths) != 2 || memberPaths["/api/objects/note"] == nil || memberPaths["/api/objects/company"] != nil {
		t.Fatalf("member paths %#v", pathKeys(memberPaths))
	}

	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "reader", "label": "Reader",
		"grants": []any{map[string]any{"object": "company", "can_read": true, "can_write": false}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var reader apiKeyView
	mustJSON(t, rec, &reader)
	keyPaths := pathItems(t, openAPIDoc(t, reader.Token))
	if len(keyPaths) != 2 || keyPaths["/api/objects/company"] == nil || keyPaths["/api/objects/note"] != nil {
		t.Fatalf("key paths %#v", pathKeys(keyPaths))
	}
	rec = apiDo(t, http.MethodPost, "/api/keys", token, map[string]any{
		"name": "writer", "label": "Writer",
		"grants": []any{map[string]any{"object": "note", "can_read": false, "can_write": true}},
	})
	requireStatus(t, rec, http.StatusCreated)
	var writer apiKeyView
	mustJSON(t, rec, &writer)
	assertEmptyPaths(t, writer.Token)
	for _, path := range []string{"/api/meta/objects", "/api/keys"} {
		rec = apiDo(t, http.MethodGet, path, reader.Token, nil)
		requireStatus(t, rec, http.StatusForbidden)
		if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
			t.Fatalf("%s %s", path, rec.Body.String())
		}
	}

	other := newOwner(t, "OpenAPI Other", "openapi-other@example.com")
	otherID := orgOf(t, other)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `delete from jobs where organization_id = $1::uuid`, otherID)
	})
	postObject(t, other, map[string]any{
		"name": "vendor", "label": "Vendor",
		"fields": []map[string]any{{"name": "name", "type": "text", "required": true}},
	})
	otherPaths := pathItems(t, openAPIDoc(t, other))
	if otherPaths["/api/objects/vendor"] == nil || otherPaths["/api/objects/company"] != nil || otherPaths["/api/objects/note"] != nil {
		t.Fatalf("other paths %#v", pathKeys(otherPaths))
	}
}

func assertCompanyFixture(t *testing.T, doc map[string]any, collection, item map[string]any) {
	t.Helper()
	if _, ok := collection["patch"]; ok || collection["delete"] != nil || collection["put"] != nil {
		t.Fatalf("collection methods %#v", collection)
	}
	if item["post"] != nil || item["put"] != nil {
		t.Fatalf("item methods %#v", item)
	}
	list := operation(t, collection, "get")
	params := parameters(t, list)
	if len(params) != 4 {
		t.Fatalf("list params %#v", params)
	}
	limit := params["limit"]["schema"].(map[string]any)
	if limit["type"] != "integer" || limit["minimum"] != float64(1) || limit["maximum"] != float64(200) || limit["default"] != float64(50) {
		t.Fatalf("limit %#v", limit)
	}
	assertRepeatable(t, params["filter"], "field:op:value")
	assertRepeatable(t, params["sort"], "field:asc or field:desc")
	view := params["view"]["schema"].(map[string]any)
	if view["type"] != "string" {
		t.Fatalf("view %#v", view)
	}
	assertStatuses(t, list, "200", false)
	listSchema := mediaSchema(t, operationResponses(t, list)["200"].(map[string]any))
	records := mapOf(t, mapOf(t, listSchema, "properties"), "records")
	if records["type"] != "array" || refOf(t, records["items"].(map[string]any)) != "#/components/schemas/company.record" {
		t.Fatalf("records %#v", records)
	}

	create := operation(t, collection, "post")
	if _, ok := create["parameters"]; ok {
		t.Fatalf("create params %#v", create["parameters"])
	}
	body := requestSchema(t, create)
	if refOf(t, body) != "#/components/schemas/company" {
		t.Fatalf("create body %#v", body)
	}
	assertStatuses(t, create, "201", false)
	if refOf(t, mediaSchema(t, operationResponses(t, create)["201"].(map[string]any))) != "#/components/schemas/company.record" {
		t.Fatal("create response")
	}

	company := schemaAt(t, doc, "company")
	props := mapOf(t, company, "properties")
	if props["name"].(map[string]any)["type"] != "string" || props["employees"].(map[string]any)["type"] != "number" {
		t.Fatalf("company props %#v", props)
	}
	industry := props["industry"].(map[string]any)
	if industry["type"] != "string" || strings.Join(jsonStrings(t, industry["enum"]), ",") != "SaaS,Finance" {
		t.Fatalf("industry %#v", industry)
	}
	if strings.Join(jsonStrings(t, company["required"]), ",") != "name" {
		t.Fatalf("required %#v", company["required"])
	}
	if company["additionalProperties"] != false {
		t.Fatalf("company additional %#v", company["additionalProperties"])
	}
	partial := schemaAt(t, doc, "company.patch")
	if _, ok := partial["required"]; ok {
		t.Fatalf("partial required %#v", partial["required"])
	}
	if partial["properties"].(map[string]any)["employees"].(map[string]any)["type"] != "number" {
		t.Fatal("partial employees")
	}
	record := schemaAt(t, doc, "company.record")
	if strings.Join(jsonStrings(t, record["required"]), ",") != "id,version,data" {
		t.Fatalf("record %#v", record)
	}
	recordProps := mapOf(t, record, "properties")
	if recordProps["id"].(map[string]any)["type"] != "string" || recordProps["version"].(map[string]any)["type"] != "integer" {
		t.Fatalf("record props %#v", recordProps)
	}
	if refOf(t, recordProps["data"].(map[string]any)) != "#/components/schemas/company" {
		t.Fatalf("record data %#v", recordProps["data"])
	}

	read := operation(t, item, "get")
	assertIDParam(t, read)
	assertStatuses(t, read, "200", false)
	patch := operation(t, item, "patch")
	assertIDParam(t, patch)
	patchSchema := requestSchema(t, patch)
	if strings.Join(jsonStrings(t, patchSchema["required"]), ",") != "version" {
		t.Fatalf("patch required %#v", patchSchema["required"])
	}
	version := mapOf(t, mapOf(t, patchSchema, "properties"), "version")
	if version["type"] != "integer" || version["minimum"] != float64(1) {
		t.Fatalf("version %#v", version)
	}
	data := mapOf(t, mapOf(t, patchSchema, "properties"), "data")
	if refOf(t, data) != "#/components/schemas/company.patch" {
		t.Fatalf("patch data %#v", data)
	}
	assertStatuses(t, patch, "200", true)
	del := operation(t, item, "delete")
	assertIDParam(t, del)
	assertStatuses(t, del, "204", false)
	deleted := operationResponses(t, del)["204"].(map[string]any)
	if _, ok := deleted["content"]; ok {
		t.Fatalf("204 %#v", deleted)
	}
}

func assertNoteTypes(t *testing.T, doc map[string]any, paths map[string]map[string]any) {
	t.Helper()
	if paths["/api/objects/note"] == nil || paths["/api/objects/note/{id}"] == nil {
		t.Fatalf("note paths %#v", pathKeys(paths))
	}
	props := mapOf(t, schemaAt(t, doc, "note"), "properties")
	if props["title"].(map[string]any)["type"] != "string" || props["amount"].(map[string]any)["type"] != "number" {
		t.Fatalf("note %#v", props)
	}
	if props["active"].(map[string]any)["type"] != "boolean" || props["opened"].(map[string]any)["type"] != "string" {
		t.Fatalf("note scalars %#v", props)
	}
	if props["account"].(map[string]any)["type"] != "string" {
		t.Fatalf("relation %#v", props["account"])
	}
	if strings.Join(jsonStrings(t, schemaAt(t, doc, "note")["required"]), ",") != "title" {
		t.Fatalf("note required %#v", schemaAt(t, doc, "note")["required"])
	}
}

func assertRepeatable(t *testing.T, param map[string]any, phrase string) {
	t.Helper()
	desc, _ := param["description"].(string)
	if !strings.Contains(desc, phrase) || param["style"] != "form" || param["explode"] != true {
		t.Fatalf("param %#v", param)
	}
	schema := param["schema"].(map[string]any)
	items, _ := schema["items"].(map[string]any)
	if schema["type"] != "array" || items["type"] != "string" {
		t.Fatalf("repeatable %#v", schema)
	}
}

func assertStatuses(t *testing.T, op map[string]any, success string, conflict bool) {
	t.Helper()
	responses := operationResponses(t, op)
	if _, ok := responses[success]; !ok {
		t.Fatalf("missing %s %#v", success, responses)
	}
	assertErrorStatus(t, responses, "401", "unauthorized")
	assertErrorStatus(t, responses, "403", "forbidden")
	assertErrorStatus(t, responses, "404", "not found")
	if conflict {
		assertErrorStatus(t, responses, "409", "stale version")
	} else if _, ok := responses["409"]; ok {
		t.Fatalf("unexpected 409 %#v", responses["409"])
	}
}

func assertErrorStatus(t *testing.T, responses map[string]any, code, msg string) {
	t.Helper()
	resp, ok := responses[code].(map[string]any)
	if !ok {
		t.Fatalf("missing %s", code)
	}
	schema := mediaSchema(t, resp)
	errProp := mapOf(t, mapOf(t, schema, "properties"), "error")
	if strings.Join(jsonStrings(t, errProp["enum"]), ",") != msg {
		t.Fatalf("%s %#v", code, errProp)
	}
}

func assertIDParam(t *testing.T, op map[string]any) {
	t.Helper()
	params := parameters(t, op)
	if len(params) != 1 || params["id"]["in"] != "path" || params["id"]["required"] != true {
		t.Fatalf("id %#v", params)
	}
	schema := params["id"]["schema"].(map[string]any)
	if schema["type"] != "string" {
		t.Fatalf("id schema %#v", schema)
	}
}

func assertBearer(t *testing.T, doc map[string]any) {
	t.Helper()
	schemes := mapOf(t, mapOf(t, doc, "components"), "securitySchemes")
	bearer := mapOf(t, schemes, "bearerAuth")
	if bearer["type"] != "http" || bearer["scheme"] != "bearer" {
		t.Fatalf("bearer %#v", bearer)
	}
	security, ok := doc["security"].([]any)
	if !ok || len(security) != 1 {
		t.Fatalf("security %#v", doc["security"])
	}
	entry, ok := security[0].(map[string]any)
	if !ok {
		t.Fatalf("security %#v", security)
	}
	scopes, ok := entry["bearerAuth"].([]any)
	if !ok || len(scopes) != 0 {
		t.Fatalf("scopes %#v", entry)
	}
}

func assertEmptyPaths(t *testing.T, token string) {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/openapi.json", token, nil)
	requireStatus(t, rec, http.StatusOK)
	var raw struct {
		OpenAPI string          `json:"openapi"`
		Paths   json.RawMessage `json:"paths"`
	}
	mustJSON(t, rec, &raw)
	if raw.OpenAPI != "3.0.3" || string(raw.Paths) != "{}" {
		t.Fatalf("empty document %s %s", raw.OpenAPI, raw.Paths)
	}
}

func openAPIDoc(t *testing.T, token string) map[string]any {
	t.Helper()
	rec := apiDo(t, http.MethodGet, "/api/openapi.json", token, nil)
	requireStatus(t, rec, http.StatusOK)
	return decodeOpenAPI(t, rec)
}

func decodeOpenAPI(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var doc map[string]any
	mustJSON(t, rec, &doc)
	return doc
}

func pathItems(t *testing.T, doc map[string]any) map[string]map[string]any {
	t.Helper()
	raw, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths %#v", doc["paths"])
	}
	out := map[string]map[string]any{}
	for key, value := range raw {
		item, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("path %s %#v", key, value)
		}
		out[key] = item
	}
	return out
}

func pathKeys(paths map[string]map[string]any) []string {
	keys := make([]string, 0, len(paths))
	for key := range paths {
		keys = append(keys, key)
	}
	return keys
}

func operation(t *testing.T, item map[string]any, method string) map[string]any {
	t.Helper()
	return mapOf(t, item, method)
}

func operationResponses(t *testing.T, op map[string]any) map[string]any {
	t.Helper()
	return mapOf(t, op, "responses")
}

func parameters(t *testing.T, op map[string]any) map[string]map[string]any {
	t.Helper()
	raw, ok := op["parameters"].([]any)
	if !ok {
		t.Fatalf("parameters %#v", op["parameters"])
	}
	out := map[string]map[string]any{}
	for _, item := range raw {
		param, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("parameter %#v", item)
		}
		name, _ := param["name"].(string)
		out[name] = param
	}
	return out
}

func requestSchema(t *testing.T, op map[string]any) map[string]any {
	t.Helper()
	body := mapOf(t, op, "requestBody")
	if body["required"] != true {
		t.Fatalf("request body %#v", body)
	}
	return mediaSchema(t, map[string]any{"content": body["content"]})
}

func mediaSchema(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	content := mapOf(t, response, "content")
	media := mapOf(t, content, "application/json")
	return mapOf(t, media, "schema")
}

func schemaAt(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	schemas := mapOf(t, mapOf(t, doc, "components"), "schemas")
	return mapOf(t, schemas, name)
}

func mapOf(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	child, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("missing %s in %#v", key, parent)
	}
	return child
}

func refOf(t *testing.T, schema map[string]any) string {
	t.Helper()
	ref, _ := schema["$ref"].(string)
	if ref == "" {
		t.Fatalf("missing ref %#v", schema)
	}
	return ref
}

func jsonStrings(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("want strings, got %#v", value)
	}
	out := make([]string, len(items))
	var okItem bool
	for i, item := range items {
		out[i], okItem = item.(string)
		if !okItem {
			t.Fatalf("want string, got %#v", item)
		}
	}
	return out
}
