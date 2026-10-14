package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type recordView struct {
	ID      string         `json:"id"`
	Version int            `json:"version"`
	Data    map[string]any `json:"data"`
}

type patchBody struct {
	Version int            `json:"version"`
	Data    map[string]any `json:"data"`
}

func (s *server) createRecord(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.authorize(r.Context(), a, objectID, true); err != nil {
		writeErr(w, err)
		return
	}
	var data map[string]any
	if err := readJSON(r, &data); err != nil {
		writeErr(w, err)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	defs, err := s.fieldDefs(r.Context(), objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := validateData(defs, data, true); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.checkRelations(r.Context(), a.OrgID, objectID, defs, data); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.insertRecord(r.Context(), a, objectID, r.PathValue("name"), defs, data, sourceAPI)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) getRecord(w http.ResponseWriter, r *http.Request) {
	view, err := s.readRecord(r, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) listRecords(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.authorize(r.Context(), a, objectID, false); err != nil {
		writeErr(w, err)
		return
	}
	defs, err := s.fieldDefs(r.Context(), objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	query, err := s.resolveList(r, a.OrgID, objectID, defs)
	if err != nil {
		writeErr(w, err)
		return
	}
	views, err := s.selectRecords(r.Context(), query)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": projectRecords(views, query.Columns)})
}

func (s *server) patchRecord(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.authorize(r.Context(), a, objectID, true); err != nil {
		writeErr(w, err)
		return
	}
	var body patchBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Version < 1 {
		writeErr(w, apiErr(http.StatusBadRequest, "version is required"))
		return
	}
	if body.Data == nil {
		body.Data = map[string]any{}
	}
	defs, err := s.fieldDefs(r.Context(), objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := validateData(defs, body.Data, false); err != nil {
		writeErr(w, err)
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())

	current, err := lockRecord(r.Context(), tx, a.OrgID, objectID, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if current.Version != body.Version {
		writeErr(w, apiErr(http.StatusConflict, "stale version"))
		return
	}
	before := map[string]any{}
	for key, value := range current.Data {
		before[key] = value
	}
	for key, value := range body.Data {
		current.Data[key] = value
	}
	if err := validateData(defs, current.Data, true); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.checkRelations(r.Context(), a.OrgID, objectID, defs, current.Data); err != nil {
		writeErr(w, err)
		return
	}
	raw, err := json.Marshal(current.Data)
	if err != nil {
		writeErr(w, err)
		return
	}
	var version int
	err = tx.QueryRow(r.Context(), `
		update records
		set data = $1::jsonb, version = version + 1, updated_at = now()
		where id = $2::uuid and organization_id = $3::uuid and version = $4 and deleted_at is null
		returning version
	`, string(raw), current.ID, a.OrgID, body.Version).Scan(&version)
	if err == pgx.ErrNoRows {
		writeErr(w, apiErr(http.StatusConflict, "stale version"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	objectName := r.PathValue("name")
	if err := insertEvent(r.Context(), tx, a.OrgID, eventWrite{
		Name:     "record.updated",
		ActorID:  a.UserID,
		ObjectID: objectID,
		RecordID: current.ID,
		Source:   sourceAPI,
		Payload: map[string]any{
			"object":  objectName,
			"version": version,
			"data":    current.Data,
		},
	}); err != nil {
		writeErr(w, err)
		return
	}
	if err := emitFieldChanges(r.Context(), tx, a.OrgID, a.UserID, objectID, current.ID, objectName, sourceAPI, fieldChanges(defs, before, body.Data)); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.committed(r.Context(), tx); err != nil {
		writeErr(w, err)
		return
	}
	current.Version = version
	writeJSON(w, http.StatusOK, current)
}

func (s *server) deleteRecord(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.authorize(r.Context(), a, objectID, true); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	tag, err := tx.Exec(r.Context(), `
		update records
		set deleted_at = now(), updated_at = now()
		where id = $1::uuid and organization_id = $2::uuid and object_id = $3::uuid and deleted_at is null
	`, r.PathValue("id"), a.OrgID, objectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if err := insertEvent(r.Context(), tx, a.OrgID, eventWrite{
		Name:     "record.deleted",
		ActorID:  a.UserID,
		ObjectID: objectID,
		RecordID: r.PathValue("id"),
		Source:   sourceAPI,
		Payload:  map[string]any{"object": r.PathValue("name")},
	}); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.committed(r.Context(), tx); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) readRecord(r *http.Request, write bool) (recordView, error) {
	a := actorFrom(r.Context())
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		return recordView{}, err
	}
	if err := s.authorize(r.Context(), a, objectID, write); err != nil {
		return recordView{}, err
	}
	return s.selectRecord(r.Context(), a.OrgID, objectID, r.PathValue("id"))
}

func (s *server) insertRecord(ctx context.Context, a actor, objectID, objectName string, defs []fieldDef, data map[string]any, source string) (recordView, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return recordView{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return recordView{}, err
	}
	defer tx.Rollback(ctx)
	id := uuid.NewString()
	_, err = tx.Exec(ctx, `
		insert into records (id, organization_id, object_id, data)
		values ($1, $2, $3, $4::jsonb)
	`, id, a.OrgID, objectID, string(raw))
	if err != nil {
		return recordView{}, err
	}
	if err := insertEvent(ctx, tx, a.OrgID, eventWrite{
		Name:     "record.created",
		ActorID:  a.UserID,
		ObjectID: objectID,
		RecordID: id,
		Source:   source,
		Payload: map[string]any{
			"object":  objectName,
			"version": 1,
			"data":    data,
		},
	}); err != nil {
		return recordView{}, err
	}
	if err := emitFieldChanges(ctx, tx, a.OrgID, a.UserID, objectID, id, objectName, source, fieldChanges(defs, map[string]any{}, data)); err != nil {
		return recordView{}, err
	}
	if err := s.committed(ctx, tx); err != nil {
		return recordView{}, err
	}
	return s.selectRecord(ctx, a.OrgID, objectID, id)
}

func (s *server) selectRecord(ctx context.Context, orgID, objectID, id string) (recordView, error) {
	if _, err := uuid.Parse(id); err != nil {
		return recordView{}, apiErr(http.StatusNotFound, "not found")
	}
	var view recordView
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select id::text, version, data
		from records
		where id = $1::uuid and organization_id = $2::uuid and object_id = $3::uuid and deleted_at is null
	`, id, orgID, objectID).Scan(&view.ID, &view.Version, &raw)
	if err == pgx.ErrNoRows {
		return recordView{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return recordView{}, err
	}
	view.Data, err = decodeData(raw)
	return view, err
}

func (s *server) selectRecords(ctx context.Context, query listQuery) ([]recordView, error) {
	sql, args := query.sql()
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []recordView{}
	for rows.Next() {
		var view recordView
		var raw []byte
		if err := rows.Scan(&view.ID, &view.Version, &raw); err != nil {
			return nil, err
		}
		view.Data, err = decodeData(raw)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func lockRecord(ctx context.Context, tx pgx.Tx, orgID, objectID, id string) (recordView, error) {
	if _, err := uuid.Parse(id); err != nil {
		return recordView{}, apiErr(http.StatusNotFound, "not found")
	}
	var view recordView
	var raw []byte
	var deleted *time.Time
	err := tx.QueryRow(ctx, `
		select id::text, version, data, deleted_at
		from records
		where id = $1::uuid and organization_id = $2::uuid and object_id = $3::uuid
		for update
	`, id, orgID, objectID).Scan(&view.ID, &view.Version, &raw, &deleted)
	if err == pgx.ErrNoRows || deleted != nil {
		return recordView{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return recordView{}, err
	}
	view.Data, err = decodeData(raw)
	return view, err
}

func decodeData(raw []byte) (map[string]any, error) {
	data := map[string]any{}
	if len(raw) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func listLimit(r *http.Request) (int, error) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 200 {
		return 0, apiErr(http.StatusBadRequest, "invalid limit")
	}
	return n, nil
}

func validateData(defs []fieldDef, data map[string]any, require bool) error {
	byName := map[string]fieldDef{}
	for _, def := range defs {
		byName[def.Name] = def
	}
	for key, value := range data {
		def, ok := byName[key]
		if !ok {
			return apiErrf(http.StatusBadRequest, "unknown field %s", key)
		}
		if err := validateValue(def, value); err != nil {
			return err
		}
	}
	if !require {
		return nil
	}
	for _, def := range defs {
		value, ok := data[def.Name]
		if !def.Required {
			continue
		}
		if !ok || value == nil || value == "" {
			return apiErrf(http.StatusBadRequest, "missing required field %s", def.Name)
		}
	}
	return nil
}

func validateValue(def fieldDef, value any) error {
	if value == nil {
		if def.Required {
			return apiErrf(http.StatusBadRequest, "missing required field %s", def.Name)
		}
		return nil
	}
	switch def.Type {
	case "text", "relation":
		text, ok := value.(string)
		if !ok {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		if def.Required && text == "" {
			return apiErrf(http.StatusBadRequest, "missing required field %s", def.Name)
		}
	case "number", "currency":
		if _, ok := value.(float64); !ok {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
	case "date":
		text, ok := value.(string)
		if !ok {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			if _, err := time.Parse("2006-01-02", text); err != nil {
				return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
			}
		}
	case "enum":
		text, ok := value.(string)
		if !ok {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		for _, option := range def.Options {
			if text == option {
				return nil
			}
		}
		return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
	default:
		return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
	}
	return nil
}
