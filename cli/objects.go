package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

var objectName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type objectFile struct {
	Name   string      `json:"name"`
	Label  string      `json:"label"`
	Fields []fieldFile `json:"fields"`
}

type fieldFile struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	To       string   `json:"to,omitempty"`
}

type differError struct {
	names []string
}

func (e *differError) Error() string {
	var b strings.Builder
	for i, name := range e.names {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "object %s differs", name)
	}
	return b.String()
}

type statusError struct {
	text string
}

func (e *statusError) Error() string { return e.text }

type apiClient struct {
	base  string
	token string
	http  *http.Client
}

func newAPI(base, token string) *apiClient {
	return &apiClient{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *apiClient) do(method, path string, in any) ([]byte, error) {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		text := strings.TrimSpace(string(raw))
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &parsed) == nil && parsed.Error != "" {
			text = parsed.Error
		}
		if text == "" {
			text = resp.Status
		}
		return nil, &statusError{text: text}
	}
	return raw, nil
}

func (c *apiClient) listObjects() ([]objectFile, error) {
	raw, err := c.do(http.MethodGet, "/api/meta/objects", nil)
	if err != nil {
		return nil, err
	}
	var body struct {
		Objects []objectFile `json:"objects"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	if body.Objects == nil {
		body.Objects = []objectFile{}
	}
	return body.Objects, nil
}

func (c *apiClient) createObject(file objectFile) error {
	if file.Fields == nil {
		file.Fields = []fieldFile{}
	}
	_, err := c.do(http.MethodPost, "/api/meta/objects", file)
	return err
}

func pull(base, token, dir string) error {
	objects, err := newAPI(base, token).listObjects()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "objects"), 0o755); err != nil {
		return err
	}
	for _, object := range objects {
		if err := writeObject(dir, object); err != nil {
			return err
		}
	}
	return nil
}

func push(base, token, dir string) error {
	local, err := readProject(dir)
	if err != nil {
		return err
	}
	api := newAPI(base, token)
	remote, err := api.listObjects()
	if err != nil {
		return err
	}
	byName := make(map[string]objectFile, len(remote))
	for _, object := range remote {
		byName[object.Name] = canonical(object)
	}
	var differs []string
	var create []objectFile
	for _, file := range local {
		live, ok := byName[file.Name]
		if !ok {
			create = append(create, file)
			continue
		}
		if !sameObject(file, live) {
			differs = append(differs, file.Name)
		}
	}
	if len(differs) > 0 {
		return &differError{names: differs}
	}
	for _, file := range create {
		if err := api.createObject(file); err != nil {
			return err
		}
	}
	return nil
}

func writeObject(dir string, live objectFile) error {
	if !objectName.MatchString(live.Name) {
		return fmt.Errorf("invalid name %s", live.Name)
	}
	file := canonical(live)
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(filepath.Join(dir, "objects", file.Name+".json"), raw, 0o644)
}

func canonical(in objectFile) objectFile {
	out := objectFile{
		Name:   in.Name,
		Label:  in.Label,
		Fields: make([]fieldFile, 0, len(in.Fields)),
	}
	for _, field := range in.Fields {
		next := fieldFile{Name: field.Name, Type: field.Type, Required: field.Required}
		if field.Type == "enum" {
			next.Options = field.Options
		}
		if field.Type == "relation" {
			next.To = field.To
		}
		out.Fields = append(out.Fields, next)
	}
	return out
}

func sameObject(a, b objectFile) bool {
	if a.Name != b.Name || a.Label != b.Label || len(a.Fields) != len(b.Fields) {
		return false
	}
	for i := range a.Fields {
		af, bf := a.Fields[i], b.Fields[i]
		if af.Name != bf.Name || af.Type != bf.Type || af.Required != bf.Required || af.To != bf.To {
			return false
		}
		if !sameList(af.Options, bf.Options) {
			return false
		}
	}
	return true
}

func sameList(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return slices.Equal(a, b)
}

func readProject(dir string) ([]objectFile, error) {
	path := filepath.Join(dir, "objects")
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("missing %s", path)
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := jsonStem(entry.Name()); !ok {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	files := []objectFile{}
	seen := map[string]struct{}{}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var file objectFile
		if err := dec.Decode(&file); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if !objectName.MatchString(file.Name) {
			return nil, fmt.Errorf("invalid name %s", file.Name)
		}
		stem, _ := jsonStem(name)
		if stem != file.Name {
			return nil, fmt.Errorf("%s name is %s", name, file.Name)
		}
		if _, ok := seen[file.Name]; ok {
			return nil, fmt.Errorf("duplicate object %s", file.Name)
		}
		seen[file.Name] = struct{}{}
		if file.Fields == nil {
			file.Fields = []fieldFile{}
		}
		files = append(files, file)
	}
	return files, nil
}

func jsonStem(name string) (string, bool) {
	const ext = ".json"
	if len(name) <= len(ext) || !strings.EqualFold(name[len(name)-len(ext):], ext) {
		return "", false
	}
	return name[:len(name)-len(ext)], true
}
