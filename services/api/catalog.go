package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type storedView struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Label    string       `json:"label"`
	Kind     string       `json:"kind"`
	Group    string       `json:"group,omitempty"`
	Columns  []string     `json:"columns"`
	Filters  []filterBody `json:"filters"`
	Sort     []sortBody   `json:"sort"`
	Version  int          `json:"version"`
	All      bool         `json:"all"`
	Resolved []string     `json:"resolved"`
}

type storedLayout struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Fields   []string `json:"fields"`
	Resolved []string `json:"resolved"`
	Version  int      `json:"version"`
}

type storedPage struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Label    string         `json:"label"`
	Document map[string]any `json:"document"`
	Version  int            `json:"version"`
}

type viewBody struct {
	Name    string       `json:"name"`
	Label   string       `json:"label"`
	Kind    string       `json:"kind"`
	Group   string       `json:"group"`
	Columns []string     `json:"columns"`
	Filters []filterBody `json:"filters"`
	Sort    []sortBody   `json:"sort"`
}

type viewPatch struct {
	Version int           `json:"version"`
	Label   *string       `json:"label"`
	Kind    *string       `json:"kind"`
	Group   *string       `json:"group"`
	Columns *[]string     `json:"columns"`
	Filters *[]filterBody `json:"filters"`
	Sort    *[]sortBody   `json:"sort"`
}

type layoutBody struct {
	Name   string   `json:"name"`
	Label  string   `json:"label"`
	Fields []string `json:"fields"`
}

type layoutPatch struct {
	Version int       `json:"version"`
	Label   *string   `json:"label"`
	Fields  *[]string `json:"fields"`
}

type pageBody struct {
	Name     string         `json:"name"`
	Label    string         `json:"label"`
	Document map[string]any `json:"document"`
}

type pagePatch struct {
	Version  int             `json:"version"`
	Label    *string         `json:"label"`
	Document *map[string]any `json:"document"`
}

func ownerOnly(a actor) error {
	if a.Role != "owner" {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	return nil
}

func (s *server) orgObject(r *http.Request) (actor, string, error) {
	a := actorFrom(r.Context())
	id, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	return a, id, err
}

func (s *server) objectForRead(r *http.Request) (actor, string, error) {
	a, id, err := s.orgObject(r)
	if err != nil {
		return a, "", err
	}
	if err := s.authorize(r.Context(), a, id, false); err != nil {
		return a, "", err
	}
	return a, id, nil
}

func insertDefaultCatalog(ctx context.Context, tx pgx.Tx, orgID, objectID string) error {
	_, err := tx.Exec(ctx, `
		insert into view_definitions
			(id, organization_id, object_id, name, label, kind, columns, filters, sort)
		values ($1, $2, $3, 'all', 'All', 'table', '["*"]', '[]', '[]')
	`, uuid.NewString(), orgID, objectID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		insert into layout_definitions
			(id, organization_id, object_id, name, label, fields)
		values ($1, $2, $3, 'record', 'Record', '["*"]')
	`, uuid.NewString(), orgID, objectID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		insert into page_definitions
			(id, organization_id, object_id, name, label, document)
		values
			($1, $2, $3, 'list', 'List', '{"kind":"list","view":"all"}'),
			($4, $2, $3, 'record', 'Record', '{"kind":"record","layout":"record","view":"all","header":"name"}')
	`, uuid.NewString(), orgID, objectID, uuid.NewString())
	return err
}

func (s *server) listViews(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.objectForRead(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	views, err := s.loadViews(r.Context(), a.OrgID, objectID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": views})
}

func (s *server) getView(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.objectForRead(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	views, err := s.loadViews(r.Context(), a.OrgID, objectID, r.PathValue("view"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(views) == 0 {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	writeJSON(w, http.StatusOK, views[0])
}

func (s *server) createView(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.orgObject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body viewBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	defs, err := s.fieldDefs(r.Context(), objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := prepareView(&body, defs); err != nil {
		writeErr(w, err)
		return
	}
	columns, filters, sorts, err := encodeView(body.Columns, body.Filters, body.Sort)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, err = s.pool.Exec(r.Context(), `
		insert into view_definitions
			(id, organization_id, object_id, name, label, kind, group_field, columns, filters, sort)
		values ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, $10::jsonb)
	`, uuid.NewString(), a.OrgID, objectID, body.Name, body.Label, body.Kind, emptyAsNil(body.Group), columns, filters, sorts)
	if err != nil {
		if isUnique(err) {
			writeErr(w, apiErr(http.StatusConflict, "view already exists"))
			return
		}
		writeErr(w, err)
		return
	}
	view, err := s.loadView(r.Context(), a.OrgID, objectID, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view.present(defs))
}

func (s *server) patchView(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.orgObject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body viewPatch
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Version < 1 {
		writeErr(w, apiErr(http.StatusBadRequest, "version is required"))
		return
	}
	defs, err := s.fieldDefs(r.Context(), objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	current, err := lockView(r.Context(), tx, a.OrgID, objectID, r.PathValue("view"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if current.Version != body.Version {
		writeErr(w, apiErr(http.StatusConflict, "stale version"))
		return
	}
	next := viewBody{
		Name: current.Name, Label: current.Label, Kind: current.Kind, Group: current.Group,
		Columns: current.Columns, Filters: current.Filters, Sort: current.Sort,
	}
	if body.Label != nil {
		next.Label = *body.Label
	}
	if body.Kind != nil {
		next.Kind = *body.Kind
	}
	if body.Group != nil {
		next.Group = *body.Group
	}
	if body.Columns != nil {
		next.Columns = *body.Columns
	}
	if body.Filters != nil {
		next.Filters = *body.Filters
	}
	if body.Sort != nil {
		next.Sort = *body.Sort
	}
	if err := prepareView(&next, defs); err != nil {
		writeErr(w, err)
		return
	}
	columns, filters, sorts, err := encodeView(next.Columns, next.Filters, next.Sort)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		update view_definitions
		set label = $1, kind = $2, group_field = $3, columns = $4::jsonb, filters = $5::jsonb, sort = $6::jsonb,
			version = version + 1, updated_at = now()
		where id = $7 and version = $8
		returning version
	`, next.Label, next.Kind, emptyAsNil(next.Group), columns, filters, sorts, current.ID, body.Version).Scan(&current.Version); err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	current.Label = next.Label
	current.Kind = next.Kind
	current.Group = next.Group
	current.Columns = next.Columns
	current.Filters = next.Filters
	current.Sort = next.Sort
	writeJSON(w, http.StatusOK, current.present(defs))
}

func prepareView(body *viewBody, defs []fieldDef) error {
	if !namePattern.MatchString(body.Name) {
		return apiErr(http.StatusBadRequest, "invalid name")
	}
	if body.Label == "" {
		return apiErr(http.StatusBadRequest, "label is required")
	}
	if body.Columns == nil {
		body.Columns = []string{"*"}
	}
	if body.Filters == nil {
		body.Filters = []filterBody{}
	}
	if body.Sort == nil {
		body.Sort = []sortBody{}
	}
	if body.Kind == "" {
		body.Kind = "table"
	}
	byName := indexFields(defs)
	switch body.Kind {
	case "table":
		if body.Group != "" {
			return apiErr(http.StatusBadRequest, "group is only valid for kanban")
		}
	case "kanban":
		def, ok := byName[body.Group]
		if !ok || def.Type != "enum" {
			return apiErr(http.StatusBadRequest, "group must be an enum")
		}
	default:
		return apiErr(http.StatusBadRequest, "invalid view")
	}
	for _, column := range body.Columns {
		if column == "*" {
			continue
		}
		def, ok := byName[column]
		if !ok || def.Type == "timestamptz" {
			return apiErrf(http.StatusBadRequest, "unknown field %s", column)
		}
	}
	if len(body.Columns) == 0 {
		return apiErr(http.StatusBadRequest, "columns are required")
	}
	probe := listQuery{Args: []any{"org", "object"}}
	for _, filter := range body.Filters {
		if err := probe.addFilter(byName, filter.Field, filter.Operator, filter.Value); err != nil {
			return err
		}
	}
	for _, sort := range body.Sort {
		if err := probe.addSort(byName, sort.Field, sort.Direction); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) loadView(ctx context.Context, orgID, objectID, name string) (storedView, error) {
	views, err := s.loadViews(ctx, orgID, objectID, name)
	if err != nil {
		return storedView{}, err
	}
	if len(views) == 0 {
		return storedView{}, apiErr(http.StatusNotFound, "not found")
	}
	return views[0], nil
}

func (s *server) loadViews(ctx context.Context, orgID, objectID, name string) ([]storedView, error) {
	query := `
		select id::text, name, label, kind, group_field, columns, filters, sort, version
		from view_definitions
		where organization_id = $1 and object_id = $2`
	args := []any{orgID, objectID}
	if name != "" {
		query += ` and name = $3`
		args = append(args, name)
	}
	query += ` order by name`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	defs, err := s.fieldDefs(ctx, objectID)
	if err != nil {
		return nil, err
	}
	views := []storedView{}
	for rows.Next() {
		view, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		views = append(views, view.present(defs))
	}
	return views, rows.Err()
}

func scanView(row pgx.Row) (storedView, error) {
	var view storedView
	var columns, filters, sorts []byte
	var group *string
	if err := row.Scan(&view.ID, &view.Name, &view.Label, &view.Kind, &group, &columns, &filters, &sorts, &view.Version); err != nil {
		return view, err
	}
	if group != nil {
		view.Group = *group
	}
	if err := json.Unmarshal(columns, &view.Columns); err != nil {
		return view, err
	}
	if err := json.Unmarshal(filters, &view.Filters); err != nil {
		return view, err
	}
	if err := json.Unmarshal(sorts, &view.Sort); err != nil {
		return view, err
	}
	if view.Filters == nil {
		view.Filters = []filterBody{}
	}
	if view.Sort == nil {
		view.Sort = []sortBody{}
	}
	return view, nil
}

func (view storedView) present(defs []fieldDef) storedView {
	view.All = hasStar(view.Columns)
	view.Resolved = resolveNames(defs, view.Columns)
	return view
}

func lockView(ctx context.Context, tx pgx.Tx, orgID, objectID, name string) (storedView, error) {
	row := tx.QueryRow(ctx, `
		select id::text, name, label, kind, group_field, columns, filters, sort, version
		from view_definitions
		where organization_id = $1 and object_id = $2 and name = $3
		for update
	`, orgID, objectID, name)
	view, err := scanView(row)
	if err == pgx.ErrNoRows {
		return storedView{}, apiErr(http.StatusNotFound, "not found")
	}
	return view, err
}

func (s *server) listLayouts(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.objectForRead(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	layouts, err := s.loadLayouts(r.Context(), a.OrgID, objectID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"layouts": layouts})
}

func (s *server) getLayout(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.objectForRead(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	layout, err := s.loadLayout(r.Context(), a.OrgID, objectID, r.PathValue("layout"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, layout)
}

func (s *server) createLayout(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.orgObject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body layoutBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	defs, err := s.fieldDefs(r.Context(), objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := prepareLayout(&body, defs); err != nil {
		writeErr(w, err)
		return
	}
	raw, err := jsonList(body.Fields)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, err = s.pool.Exec(r.Context(), `
		insert into layout_definitions (id, organization_id, object_id, name, label, fields)
		values ($1, $2, $3, $4, $5, $6::jsonb)
	`, uuid.NewString(), a.OrgID, objectID, body.Name, body.Label, raw)
	if err != nil {
		if isUnique(err) {
			writeErr(w, apiErr(http.StatusConflict, "layout already exists"))
			return
		}
		writeErr(w, err)
		return
	}
	layout, err := s.loadLayout(r.Context(), a.OrgID, objectID, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, layout)
}

func (s *server) patchLayout(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.orgObject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body layoutPatch
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Version < 1 {
		writeErr(w, apiErr(http.StatusBadRequest, "version is required"))
		return
	}
	defs, err := s.fieldDefs(r.Context(), objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	current, err := lockLayout(r.Context(), tx, a.OrgID, objectID, r.PathValue("layout"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if current.Version != body.Version {
		writeErr(w, apiErr(http.StatusConflict, "stale version"))
		return
	}
	next := layoutBody{Name: current.Name, Label: current.Label, Fields: current.Fields}
	if body.Label != nil {
		next.Label = *body.Label
	}
	if body.Fields != nil {
		next.Fields = *body.Fields
	}
	if err := prepareLayout(&next, defs); err != nil {
		writeErr(w, err)
		return
	}
	raw, err := jsonList(next.Fields)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		update layout_definitions
		set label = $1, fields = $2::jsonb, version = version + 1, updated_at = now()
		where id = $3 and version = $4
		returning version
	`, next.Label, raw, current.ID, body.Version).Scan(&current.Version); err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	current.Label = next.Label
	current.Fields = next.Fields
	current.Resolved = resolveNames(defs, next.Fields)
	writeJSON(w, http.StatusOK, current)
}

func prepareLayout(body *layoutBody, defs []fieldDef) error {
	if !namePattern.MatchString(body.Name) {
		return apiErr(http.StatusBadRequest, "invalid name")
	}
	if body.Label == "" {
		return apiErr(http.StatusBadRequest, "label is required")
	}
	if body.Fields == nil {
		body.Fields = []string{"*"}
	}
	if len(body.Fields) == 0 {
		return apiErr(http.StatusBadRequest, "fields are required")
	}
	byName := indexFields(defs)
	for _, field := range body.Fields {
		if field == "*" {
			continue
		}
		def, ok := byName[field]
		if !ok || def.Type == "timestamptz" {
			return apiErrf(http.StatusBadRequest, "unknown field %s", field)
		}
	}
	return nil
}

func (s *server) loadLayout(ctx context.Context, orgID, objectID, name string) (storedLayout, error) {
	layouts, err := s.loadLayouts(ctx, orgID, objectID, name)
	if err != nil {
		return storedLayout{}, err
	}
	if len(layouts) == 0 {
		return storedLayout{}, apiErr(http.StatusNotFound, "not found")
	}
	return layouts[0], nil
}

func (s *server) loadLayouts(ctx context.Context, orgID, objectID, name string) ([]storedLayout, error) {
	query := `
		select id::text, name, label, fields, version
		from layout_definitions
		where organization_id = $1 and object_id = $2`
	args := []any{orgID, objectID}
	if name != "" {
		query += ` and name = $3`
		args = append(args, name)
	}
	query += ` order by name`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	defs, err := s.fieldDefs(ctx, objectID)
	if err != nil {
		return nil, err
	}
	layouts := []storedLayout{}
	for rows.Next() {
		var layout storedLayout
		var raw []byte
		if err := rows.Scan(&layout.ID, &layout.Name, &layout.Label, &raw, &layout.Version); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &layout.Fields); err != nil {
			return nil, err
		}
		layout.Resolved = resolveNames(defs, layout.Fields)
		layouts = append(layouts, layout)
	}
	return layouts, rows.Err()
}

func lockLayout(ctx context.Context, tx pgx.Tx, orgID, objectID, name string) (storedLayout, error) {
	var layout storedLayout
	var raw []byte
	err := tx.QueryRow(ctx, `
		select id::text, name, label, fields, version
		from layout_definitions
		where organization_id = $1 and object_id = $2 and name = $3
		for update
	`, orgID, objectID, name).Scan(&layout.ID, &layout.Name, &layout.Label, &raw, &layout.Version)
	if err == pgx.ErrNoRows {
		return storedLayout{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return storedLayout{}, err
	}
	if err := json.Unmarshal(raw, &layout.Fields); err != nil {
		return storedLayout{}, err
	}
	return layout, nil
}

func (s *server) listPages(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.objectForRead(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	pages, err := s.loadPages(r.Context(), a.OrgID, objectID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": pages})
}

func (s *server) getPage(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.objectForRead(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	page, err := s.loadPage(r.Context(), a.OrgID, objectID, r.PathValue("page"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *server) createPage(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.orgObject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body pageBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.preparePage(r.Context(), a.OrgID, objectID, &body); err != nil {
		writeErr(w, err)
		return
	}
	raw, err := json.Marshal(body.Document)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, err = s.pool.Exec(r.Context(), `
		insert into page_definitions (id, organization_id, object_id, name, label, document)
		values ($1, $2, $3, $4, $5, $6::jsonb)
	`, uuid.NewString(), a.OrgID, objectID, body.Name, body.Label, string(raw))
	if err != nil {
		if isUnique(err) {
			writeErr(w, apiErr(http.StatusConflict, "page already exists"))
			return
		}
		writeErr(w, err)
		return
	}
	page, err := s.loadPage(r.Context(), a.OrgID, objectID, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, page)
}

func (s *server) patchPage(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.orgObject(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body pagePatch
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Version < 1 {
		writeErr(w, apiErr(http.StatusBadRequest, "version is required"))
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	current, err := lockPage(r.Context(), tx, a.OrgID, objectID, r.PathValue("page"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if current.Version != body.Version {
		writeErr(w, apiErr(http.StatusConflict, "stale version"))
		return
	}
	next := pageBody{Name: current.Name, Label: current.Label, Document: current.Document}
	if body.Label != nil {
		next.Label = *body.Label
	}
	if body.Document != nil {
		next.Document = *body.Document
	}
	if err := s.preparePage(r.Context(), a.OrgID, objectID, &next); err != nil {
		writeErr(w, err)
		return
	}
	raw, err := json.Marshal(next.Document)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		update page_definitions
		set label = $1, document = $2::jsonb, version = version + 1, updated_at = now()
		where id = $3 and version = $4
		returning version
	`, next.Label, string(raw), current.ID, body.Version).Scan(&current.Version); err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	current.Label = next.Label
	current.Document = next.Document
	writeJSON(w, http.StatusOK, current)
}

func (s *server) preparePage(ctx context.Context, orgID, objectID string, body *pageBody) error {
	if !namePattern.MatchString(body.Name) {
		return apiErr(http.StatusBadRequest, "invalid name")
	}
	if body.Label == "" {
		return apiErr(http.StatusBadRequest, "label is required")
	}
	if body.Document == nil {
		return apiErr(http.StatusBadRequest, "invalid page")
	}
	kind, _ := body.Document["kind"].(string)
	switch kind {
	case "list", "record":
	default:
		return apiErr(http.StatusBadRequest, "invalid page")
	}
	if view, _ := body.Document["view"].(string); view != "" {
		if _, err := s.loadView(ctx, orgID, objectID, view); err != nil {
			return apiErr(http.StatusBadRequest, "invalid page")
		}
	} else if kind == "list" {
		return apiErr(http.StatusBadRequest, "invalid page")
	}
	if layout, _ := body.Document["layout"].(string); layout != "" {
		if _, err := s.loadLayout(ctx, orgID, objectID, layout); err != nil {
			return apiErr(http.StatusBadRequest, "invalid page")
		}
	} else if kind == "record" {
		return apiErr(http.StatusBadRequest, "invalid page")
	}
	return nil
}

func (s *server) loadPage(ctx context.Context, orgID, objectID, name string) (storedPage, error) {
	pages, err := s.loadPages(ctx, orgID, objectID, name)
	if err != nil {
		return storedPage{}, err
	}
	if len(pages) == 0 {
		return storedPage{}, apiErr(http.StatusNotFound, "not found")
	}
	return pages[0], nil
}

func (s *server) loadPages(ctx context.Context, orgID, objectID, name string) ([]storedPage, error) {
	query := `
		select id::text, name, label, document, version
		from page_definitions
		where organization_id = $1 and object_id = $2`
	args := []any{orgID, objectID}
	if name != "" {
		query += ` and name = $3`
		args = append(args, name)
	}
	query += ` order by name`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := []storedPage{}
	for rows.Next() {
		var page storedPage
		var raw []byte
		if err := rows.Scan(&page.ID, &page.Name, &page.Label, &raw, &page.Version); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &page.Document); err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, rows.Err()
}

func lockPage(ctx context.Context, tx pgx.Tx, orgID, objectID, name string) (storedPage, error) {
	var page storedPage
	var raw []byte
	err := tx.QueryRow(ctx, `
		select id::text, name, label, document, version
		from page_definitions
		where organization_id = $1 and object_id = $2 and name = $3
		for update
	`, orgID, objectID, name).Scan(&page.ID, &page.Name, &page.Label, &raw, &page.Version)
	if err == pgx.ErrNoRows {
		return storedPage{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return storedPage{}, err
	}
	if err := json.Unmarshal(raw, &page.Document); err != nil {
		return storedPage{}, err
	}
	return page, nil
}

func (s *server) listRelationships(w http.ResponseWriter, r *http.Request) {
	a, objectID, err := s.objectForRead(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		select name, kind, to_name, inverse_name
		from (
			select d.name, d.kind, o.name as to_name, d.inverse_name
			from relationship_definitions d
			join object_definitions o on o.id = d.to_object_id
			where d.organization_id = $1 and d.from_object_id = $2
			union all
			select d.inverse_name, d.kind, o.name, d.name
			from relationship_definitions d
			join object_definitions o on o.id = d.from_object_id
			where d.organization_id = $1 and d.to_object_id = $2
			  and d.inverse_name is not null
		) relationships
		order by name
	`, a.OrgID, objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	list := []map[string]string{}
	for rows.Next() {
		var name, kind, to string
		var inverse *string
		if err := rows.Scan(&name, &kind, &to, &inverse); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]string{"name": name, "kind": kind, "to": to}
		if inverse != nil {
			item["inverse"] = *inverse
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"relationships": list})
}

func resolveNames(defs []fieldDef, names []string) []string {
	if hasStar(names) {
		resolved := make([]string, 0, len(defs))
		for _, def := range defs {
			resolved = append(resolved, def.Name)
		}
		return resolved
	}
	if names == nil {
		return []string{}
	}
	return names
}

func encodeView(columns []string, filters []filterBody, sorts []sortBody) (string, string, string, error) {
	columnsJSON, err := jsonList(columns)
	if err != nil {
		return "", "", "", err
	}
	filtersJSON, err := jsonList(filters)
	if err != nil {
		return "", "", "", err
	}
	sortsJSON, err := jsonList(sorts)
	if err != nil {
		return "", "", "", err
	}
	return columnsJSON, filtersJSON, sortsJSON, nil
}

func jsonList(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
