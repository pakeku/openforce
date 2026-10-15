package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestMissingFlagExitsTwo(t *testing.T) {
	cases := []struct {
		args []string
		flag string
	}{
		{[]string{"pull", "-token", "t", "-dir", "d"}, "-url"},
		{[]string{"pull", "-url", "u", "-dir", "d"}, "-token"},
		{[]string{"pull", "-url", "u", "-token", "t"}, "-dir"},
		{[]string{"push", "-token", "t", "-dir", "d"}, "-url"},
		{[]string{"push", "-url", "u", "-dir", "d"}, "-token"},
		{[]string{"push", "-url", "u", "-token", "t"}, "-dir"},
	}
	for _, tc := range cases {
		var stderr bytes.Buffer
		code := run(tc.args, &stderr)
		if code != 2 {
			t.Fatalf("%v exit %d stderr %s", tc.args, code, stderr.String())
		}
		if !strings.Contains(stderr.String(), tc.flag) {
			t.Fatalf("%v stderr %q", tc.args, stderr.String())
		}
	}
}

func TestAPIErrorExitsOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer srv.Close()

	var stderr bytes.Buffer
	code := run([]string{"pull", "-url", srv.URL, "-token", "t", "-dir", t.TempDir()}, &stderr)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if stderr.String() != "unavailable\n" {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestPullTwoObjectsAndSecondPushDoesNotPost(t *testing.T) {
	api := newFakeMeta("test-token")
	api.objects["company"] = objectFile{
		Name:  "company",
		Label: "Company",
		Fields: []fieldFile{
			{Name: "name", Type: "text", Required: true},
			{Name: "industry", Type: "enum", Required: false, Options: []string{"SaaS", "Finance"}},
		},
	}
	api.objects["person"] = objectFile{
		Name:  "person",
		Label: "Person",
		Fields: []fieldFile{
			{Name: "name", Type: "text", Required: true},
		},
	}
	srv := httptest.NewServer(api)
	defer srv.Close()

	dir := t.TempDir()
	var stderr bytes.Buffer
	code := run([]string{"pull", "-url", srv.URL, "-token", "test-token", "-dir", dir}, &stderr)
	if code != 0 {
		t.Fatalf("pull exit %d stderr %s", code, stderr.String())
	}
	entries, err := os.ReadDir(filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("files %d", len(entries))
	}

	companyRaw, err := os.ReadFile(filepath.Join(dir, "objects", "company.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(companyRaw), "position") || strings.Contains(string(companyRaw), `"id"`) {
		t.Fatalf("company file kept server fields: %s", companyRaw)
	}
	var company objectFile
	if err := json.Unmarshal(companyRaw, &company); err != nil {
		t.Fatal(err)
	}
	if company.Name != "company" || company.Label != "Company" || len(company.Fields) != 2 {
		t.Fatalf("company %#v", company)
	}
	if company.Fields[0].Name != "name" || company.Fields[0].Type != "text" || !company.Fields[0].Required || company.Fields[0].Options != nil {
		t.Fatalf("name field %#v", company.Fields[0])
	}
	industry := company.Fields[1]
	if industry.Name != "industry" || industry.Type != "enum" || industry.Required || strings.Join(industry.Options, ",") != "SaaS,Finance" {
		t.Fatalf("industry %#v", industry)
	}

	personRaw, err := os.ReadFile(filepath.Join(dir, "objects", "person.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(personRaw), "options") || strings.Contains(string(personRaw), "position") {
		t.Fatalf("person file %s", personRaw)
	}

	stderr.Reset()
	code = run([]string{"push", "-url", srv.URL, "-token", "test-token", "-dir", dir}, &stderr)
	if code != 0 {
		t.Fatalf("push exit %d stderr %s", code, stderr.String())
	}
	stderr.Reset()
	code = run([]string{"push", "-url", srv.URL, "-token", "test-token", "-dir", dir}, &stderr)
	if code != 0 {
		t.Fatalf("second push exit %d stderr %s", code, stderr.String())
	}
	if api.Posts() != 0 {
		t.Fatalf("posts %d", api.Posts())
	}
	if api.Deletes() != 0 {
		t.Fatalf("deletes %d", api.Deletes())
	}
}

func TestPushCreatesOnceAndLeavesOtherObjects(t *testing.T) {
	api := newFakeMeta("test-token")
	api.objects["note"] = objectFile{
		Name:  "note",
		Label: "Note",
		Fields: []fieldFile{
			{Name: "body", Type: "text", Required: false},
		},
	}
	srv := httptest.NewServer(api)
	defer srv.Close()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	deal := objectFile{
		Name:  "deal",
		Label: "Deal",
		Fields: []fieldFile{
			{Name: "stage", Type: "enum", Required: true, Options: []string{"open", "won"}},
		},
	}
	raw, err := json.MarshalIndent(deal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "objects", "deal.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := run([]string{"push", "-url", srv.URL, "-token", "test-token", "-dir", dir}, &stderr)
	if code != 0 {
		t.Fatalf("push exit %d stderr %s", code, stderr.String())
	}
	if api.Posts() != 1 {
		t.Fatalf("posts %d", api.Posts())
	}
	if _, ok := api.objects["note"]; !ok {
		t.Fatal("deleted note")
	}
	if api.Deletes() != 0 {
		t.Fatalf("deletes %d", api.Deletes())
	}
	if !strings.Contains(string(api.lastBody), `"options":["open","won"]`) {
		t.Fatalf("body %s", api.lastBody)
	}
	if strings.Contains(string(api.lastBody), "position") || strings.Contains(string(api.lastBody), `"id"`) {
		t.Fatalf("body %s", api.lastBody)
	}

	stderr.Reset()
	code = run([]string{"push", "-url", srv.URL, "-token", "test-token", "-dir", dir}, &stderr)
	if code != 0 {
		t.Fatalf("second push exit %d stderr %s", code, stderr.String())
	}
	if api.Posts() != 1 {
		t.Fatalf("second push posts %d", api.Posts())
	}
}

func TestChangedFileFailsPush(t *testing.T) {
	api := newFakeMeta("test-token")
	api.objects["company"] = objectFile{
		Name:  "company",
		Label: "Company",
		Fields: []fieldFile{
			{Name: "name", Type: "text", Required: true},
		},
	}
	srv := httptest.NewServer(api)
	defer srv.Close()

	dir := t.TempDir()
	var stderr bytes.Buffer
	code := run([]string{"pull", "-url", srv.URL, "-token", "test-token", "-dir", dir}, &stderr)
	if code != 0 {
		t.Fatalf("pull exit %d stderr %s", code, stderr.String())
	}
	path := filepath.Join(dir, "objects", "company.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file objectFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	file.Label = "Companies"
	rewritten, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(rewritten, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	deal := objectFile{Name: "deal", Label: "Deal", Fields: []fieldFile{}}
	dealRaw, err := json.MarshalIndent(deal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "objects", "deal.json"), append(dealRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr.Reset()
	code = run([]string{"push", "-url", srv.URL, "-token", "test-token", "-dir", dir}, &stderr)
	if code == 0 {
		t.Fatal("push succeeded")
	}
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "company") {
		t.Fatalf("stderr %q", stderr.String())
	}
	if api.Posts() != 0 {
		t.Fatalf("posts %d", api.Posts())
	}
	if _, ok := api.objects["deal"]; ok {
		t.Fatal("created deal despite differ")
	}
}

type fakeMeta struct {
	mu       sync.Mutex
	token    string
	objects  map[string]objectFile
	posts    int
	deletes  int
	lastBody []byte
}

func newFakeMeta(token string) *fakeMeta {
	return &fakeMeta{token: token, objects: map[string]objectFile{}}
}

func (f *fakeMeta) Posts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts
}

func (f *fakeMeta) Deletes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletes
}

func (f *fakeMeta) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		writeFakeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method == http.MethodDelete {
		f.mu.Lock()
		f.deletes++
		f.mu.Unlock()
		writeFakeErr(w, http.StatusNotFound, "not found")
		return
	}
	if r.URL.Path != "/api/meta/objects" {
		writeFakeErr(w, http.StatusNotFound, "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		f.serveList(w)
	case http.MethodPost:
		f.serveCreate(w, r)
	default:
		writeFakeErr(w, http.StatusMethodNotAllowed, "method")
	}
}

func (f *fakeMeta) serveList(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.objects))
	for name := range f.objects {
		names = append(names, name)
	}
	sort.Strings(names)
	objects := make([]map[string]any, 0, len(names))
	for i, name := range names {
		file := f.objects[name]
		fields := make([]map[string]any, 0, len(file.Fields))
		for pos, field := range file.Fields {
			item := map[string]any{
				"name":     field.Name,
				"type":     field.Type,
				"required": field.Required,
				"position": pos,
			}
			if field.Type == "enum" {
				item["options"] = field.Options
			}
			fields = append(fields, item)
		}
		objects = append(objects, map[string]any{
			"id":     fmt.Sprintf("id-%d", i),
			"name":   file.Name,
			"label":  file.Label,
			"fields": fields,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
}

func (f *fakeMeta) serveCreate(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeFakeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var file objectFile
	if err := dec.Decode(&file); err != nil {
		writeFakeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts++
	f.lastBody = append([]byte(nil), raw...)
	if _, ok := f.objects[file.Name]; ok {
		writeFakeErr(w, http.StatusConflict, "object already exists")
		return
	}
	if file.Fields == nil {
		file.Fields = []fieldFile{}
	}
	f.objects[file.Name] = file
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": "new", "name": file.Name})
}

func TestCanonicalKeepsRelationTarget(t *testing.T) {
	live := canonical(objectFile{
		Name:  "person",
		Label: "Person",
		Fields: []fieldFile{
			{Name: "name", Type: "text", To: "company"},
			{Name: "employer", Type: "relation", Required: false, To: "company"},
		},
	})
	if live.Fields[0].To != "" || live.Fields[1].To != "company" {
		t.Fatalf("canonical %#v", live.Fields)
	}
	other := live
	other.Fields = append([]fieldFile(nil), live.Fields...)
	other.Fields[1].To = "account"
	if sameObject(live, other) {
		t.Fatal("a different link target compared equal")
	}
	if !sameObject(live, live) {
		t.Fatal("the same link target compared different")
	}
}

func writeFakeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
