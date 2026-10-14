package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type fieldDef struct {
	ID       string
	Name     string
	Type     string
	Required bool
	Options  []string
	Position int
}

type fieldBody struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options"`
}

type objectBody struct {
	Name   string      `json:"name"`
	Label  string      `json:"label"`
	Fields []fieldBody `json:"fields"`
}

type fieldView struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	Position int      `json:"position"`
}

type objectView struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Label  string      `json:"label"`
	Fields []fieldView `json:"fields"`
}

func (s *server) createObject(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if a.Role != "owner" {
		writeErr(w, apiErr(http.StatusForbidden, "forbidden"))
		return
	}
	var body objectBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateObjectBody(body); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.insertObject(r.Context(), a, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listObjects(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	views, err := s.loadObjects(r.Context(), a.OrgID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"objects": views})
}

func (s *server) getObject(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	views, err := s.loadObjects(r.Context(), a.OrgID, r.PathValue("name"))
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

func validateObjectBody(body objectBody) error {
	if !namePattern.MatchString(body.Name) {
		return apiErr(http.StatusBadRequest, "invalid name")
	}
	if body.Label == "" {
		return apiErr(http.StatusBadRequest, "label is required")
	}
	seen := map[string]struct{}{}
	for _, field := range body.Fields {
		if !namePattern.MatchString(field.Name) {
			return apiErr(http.StatusBadRequest, "invalid name")
		}
		if _, ok := seen[field.Name]; ok {
			return apiErr(http.StatusBadRequest, "duplicate field")
		}
		seen[field.Name] = struct{}{}
		if field.Type == "relation" || field.Type == "formula" {
			return apiErr(http.StatusBadRequest, "relation and formula are not accepted here")
		}
		switch field.Type {
		case "text", "number", "currency", "date", "boolean":
			if len(field.Options) > 0 {
				return apiErr(http.StatusBadRequest, "options are only valid for enum")
			}
		case "enum":
			if len(field.Options) == 0 {
				return apiErr(http.StatusBadRequest, "enum requires options")
			}
			for _, option := range field.Options {
				if option == "" {
					return apiErr(http.StatusBadRequest, "enum requires options")
				}
			}
		default:
			return apiErr(http.StatusBadRequest, "invalid field type")
		}
	}
	return nil
}

func (s *server) insertObject(ctx context.Context, a actor, body objectBody) (objectView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return objectView{}, err
	}
	defer tx.Rollback(ctx)

	objectID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		insert into object_definitions (id, organization_id, name, label)
		values ($1, $2, $3, $4)
	`, objectID, a.OrgID, body.Name, body.Label)
	if err != nil {
		if isUnique(err) {
			return objectView{}, apiErr(http.StatusConflict, "object already exists")
		}
		return objectView{}, err
	}
	for i, field := range body.Fields {
		if err := insertField(ctx, tx, a.OrgID, objectID, field.Name, field.Type, field.Required, field.Options, i); err != nil {
			return objectView{}, err
		}
	}
	if err := insertDefaultCatalog(ctx, tx, a.OrgID, objectID); err != nil {
		return objectView{}, err
	}
	if _, err := insertEvent(ctx, tx, a.OrgID, eventWrite{
		Name:     "object.created",
		ActorID:  a.UserID,
		ObjectID: objectID,
		Source:   sourceAPI,
		Payload:  map[string]any{"name": body.Name, "label": body.Label},
	}); err != nil {
		return objectView{}, err
	}
	if err := s.committed(ctx, tx); err != nil {
		return objectView{}, err
	}
	views, err := s.loadObjects(ctx, a.OrgID, body.Name)
	if err != nil {
		return objectView{}, err
	}
	return views[0], nil
}

func insertField(ctx context.Context, tx pgx.Tx, orgID, objectID, name, fieldType string, required bool, options []string, position int) error {
	var encoded any
	if options != nil {
		raw, err := json.Marshal(options)
		if err != nil {
			return err
		}
		encoded = string(raw)
	}
	_, err := tx.Exec(ctx, `
		insert into field_definitions
			(id, organization_id, object_id, name, field_type, required, options, position)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
	`, uuid.NewString(), orgID, objectID, name, fieldType, required, encoded, position)
	return err
}

func (s *server) loadObjects(ctx context.Context, orgID, name string) ([]objectView, error) {
	query := `
		select id::text, name, label
		from object_definitions
		where organization_id = $1
	`
	args := []any{orgID}
	if name != "" {
		query += ` and name = $2`
		args = append(args, name)
	}
	query += ` order by name`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []objectView
	for rows.Next() {
		var view objectView
		if err := rows.Scan(&view.ID, &view.Name, &view.Label); err != nil {
			return nil, err
		}
		view.Fields = []fieldView{}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if views == nil {
		views = []objectView{}
	}
	for i := range views {
		fields, err := s.loadFields(ctx, views[i].ID)
		if err != nil {
			return nil, err
		}
		views[i].Fields = fields
	}
	return views, nil
}

func (s *server) loadFields(ctx context.Context, objectID string) ([]fieldView, error) {
	rows, err := s.pool.Query(ctx, `
		select name, field_type, required, options, position
		from field_definitions
		where object_id = $1
		order by position, name
	`, objectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fields := []fieldView{}
	for rows.Next() {
		var field fieldView
		var raw []byte
		if err := rows.Scan(&field.Name, &field.Type, &field.Required, &raw, &field.Position); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &field.Options); err != nil {
				return nil, err
			}
		}
		fields = append(fields, field)
	}
	return fields, rows.Err()
}

func (s *server) objectByName(ctx context.Context, orgID, name string) (id string, err error) {
	err = s.pool.QueryRow(ctx, `
		select id::text from object_definitions
		where organization_id = $1 and name = $2
	`, orgID, name).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", apiErr(http.StatusNotFound, "not found")
	}
	return id, err
}

func (s *server) fieldDefs(ctx context.Context, objectID string) ([]fieldDef, error) {
	rows, err := s.pool.Query(ctx, `
		select id::text, name, field_type, required, options, position
		from field_definitions
		where object_id = $1
		order by position, name
	`, objectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var defs []fieldDef
	for rows.Next() {
		var def fieldDef
		var raw []byte
		if err := rows.Scan(&def.ID, &def.Name, &def.Type, &def.Required, &raw, &def.Position); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &def.Options); err != nil {
				return nil, err
			}
		}
		defs = append(defs, def)
	}
	return defs, rows.Err()
}
