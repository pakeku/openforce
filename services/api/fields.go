package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type storedField struct {
	Name     string
	Type     string
	Required bool
	Options  []string
	Position int
	To       string
}

func (s *server) createField(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body fieldBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateField(body); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.addField(r.Context(), a.OrgID, objectID, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) patchField(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body fieldBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateField(body); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.updateField(r.Context(), a.OrgID, objectID, r.PathValue("field"), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) deleteField(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.removeField(r.Context(), a.OrgID, objectID, r.PathValue("field")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) addField(ctx context.Context, orgID, objectID string, body fieldBody) (fieldView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fieldView{}, err
	}
	defer tx.Rollback(ctx)

	var position int
	if err := tx.QueryRow(ctx, `
		select coalesce(max(position) + 1, 0) from field_definitions where object_id = $1
	`, objectID).Scan(&position); err != nil {
		return fieldView{}, err
	}
	options := body.Options
	if body.Type != "enum" {
		options = nil
	}
	if err := insertField(ctx, tx, orgID, objectID, body.Name, body.Type, body.Required, options, position); err != nil {
		if isUnique(err) {
			return fieldView{}, apiErr(http.StatusConflict, "field already exists")
		}
		return fieldView{}, err
	}
	if body.Type == "relation" {
		targetID, err := objectIDTx(ctx, tx, orgID, body.To)
		if err != nil {
			return fieldView{}, err
		}
		if err := insertBelongsTo(ctx, tx, orgID, objectID, targetID, body.Name); err != nil {
			return fieldView{}, err
		}
	}
	if err := s.committed(ctx, tx); err != nil {
		return fieldView{}, err
	}
	return presentField(body.Name, body.Type, body.Required, body.Options, position, body.To), nil
}

func (s *server) updateField(ctx context.Context, orgID, objectID, name string, body fieldBody) (fieldView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fieldView{}, err
	}
	defer tx.Rollback(ctx)

	current, err := findFieldTx(ctx, tx, objectID, name)
	if err != nil {
		return fieldView{}, err
	}
	var targetID string
	if body.Type == "relation" {
		targetID, err = objectIDTx(ctx, tx, orgID, body.To)
		if err != nil {
			return fieldView{}, err
		}
	}
	// A rename keeps stored values. A new type, or a new link target, drops only this field.
	rename := body.Name != current.Name
	typeChanged := body.Type != current.Type
	targetChanged := current.Type == "relation" && body.Type == "relation" && body.To != current.To
	if rename && !typeChanged && !targetChanged {
		if err := moveFieldValues(ctx, tx, orgID, objectID, current.Name, body.Name); err != nil {
			return fieldView{}, err
		}
	} else if typeChanged || targetChanged {
		if err := clearFieldValues(ctx, tx, orgID, objectID, current.Name); err != nil {
			return fieldView{}, err
		}
	}
	encoded, err := fieldOptions(body)
	if err != nil {
		return fieldView{}, err
	}
	tag, err := tx.Exec(ctx, `
		update field_definitions
		set name = $1, field_type = $2, required = $3, options = $4
		where object_id = $5 and name = $6
	`, body.Name, body.Type, body.Required, encoded, objectID, current.Name)
	if err != nil {
		if isUnique(err) {
			return fieldView{}, apiErr(http.StatusConflict, "field already exists")
		}
		return fieldView{}, err
	}
	if tag.RowsAffected() == 0 {
		return fieldView{}, apiErr(http.StatusNotFound, "not found")
	}
	if err := syncFieldRelation(ctx, tx, orgID, objectID, current, body, targetID); err != nil {
		return fieldView{}, err
	}
	if err := s.committed(ctx, tx); err != nil {
		return fieldView{}, err
	}
	return presentField(body.Name, body.Type, body.Required, body.Options, current.Position, body.To), nil
}

func (s *server) removeField(ctx context.Context, orgID, objectID, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := findFieldTx(ctx, tx, objectID, name); err != nil {
		return err
	}
	if err := clearFieldValues(ctx, tx, orgID, objectID, name); err != nil {
		return err
	}
	if err := deleteFieldRelationships(ctx, tx, orgID, objectID, name); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		delete from field_definitions where object_id = $1 and name = $2
	`, objectID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apiErr(http.StatusNotFound, "not found")
	}
	return s.committed(ctx, tx)
}

func syncFieldRelation(ctx context.Context, tx pgx.Tx, orgID, objectID string, current storedField, body fieldBody, targetID string) error {
	rename := body.Name != current.Name
	targetChanged := current.Type == "relation" && body.Type == "relation" && body.To != current.To
	switch {
	case current.Type == "relation" && body.Type == "relation":
		if targetChanged {
			if err := deleteInverseLists(ctx, tx, orgID, objectID, current.Name); err != nil {
				return err
			}
		}
		if err := retargetBelongsTo(ctx, tx, orgID, objectID, current.Name, body.Name, targetID); err != nil {
			return err
		}
		if rename && !targetChanged {
			return renameInverse(ctx, tx, orgID, objectID, current.Name, body.Name)
		}
		return nil
	case current.Type == "relation":
		return deleteFieldRelationships(ctx, tx, orgID, objectID, current.Name)
	case body.Type == "relation":
		return insertBelongsTo(ctx, tx, orgID, objectID, targetID, body.Name)
	default:
		return nil
	}
}

func presentField(name, fieldType string, required bool, options []string, position int, to string) fieldView {
	view := fieldView{Name: name, Type: fieldType, Required: required, Position: position}
	if fieldType == "enum" {
		view.Options = options
	}
	if fieldType == "relation" {
		view.To = to
	}
	return view
}

func fieldOptions(field fieldBody) (any, error) {
	if field.Type != "enum" {
		return nil, nil
	}
	raw, err := json.Marshal(field.Options)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func findFieldTx(ctx context.Context, tx pgx.Tx, objectID, name string) (storedField, error) {
	var field storedField
	var raw []byte
	var target *string
	err := tx.QueryRow(ctx, `
		select f.name, f.field_type, f.required, f.options, f.position, tgt.name
		from field_definitions f
		left join relationship_definitions r
		  on r.from_object_id = f.object_id
		 and r.name = f.name
		 and r.kind = 'belongs_to'
		left join object_definitions tgt on tgt.id = r.to_object_id
		where f.object_id = $1 and f.name = $2
	`, objectID, name).Scan(&field.Name, &field.Type, &field.Required, &raw, &field.Position, &target)
	if err == pgx.ErrNoRows {
		return storedField{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return storedField{}, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &field.Options); err != nil {
			return storedField{}, err
		}
	}
	if target != nil {
		field.To = *target
	}
	return field, nil
}

func objectIDTx(ctx context.Context, tx pgx.Tx, orgID, name string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		select id::text from object_definitions
		where organization_id = $1 and name = $2
	`, orgID, name).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", apiErr(http.StatusNotFound, "not found")
	}
	return id, err
}

func insertBelongsTo(ctx context.Context, tx pgx.Tx, orgID, fromID, toID, name string) error {
	_, err := tx.Exec(ctx, `
		insert into relationship_definitions
			(id, organization_id, from_object_id, to_object_id, name, kind, inverse_name)
		values ($1, $2, $3, $4, $5, 'belongs_to', null)
	`, uuid.NewString(), orgID, fromID, toID, name)
	if isUnique(err) {
		return apiErr(http.StatusConflict, "relationship already exists")
	}
	return err
}

func retargetBelongsTo(ctx context.Context, tx pgx.Tx, orgID, objectID, from, to, targetID string) error {
	tag, err := tx.Exec(ctx, `
		update relationship_definitions
		set name = $4, to_object_id = $5
		where organization_id = $1
		  and from_object_id = $2
		  and name = $3
		  and kind = 'belongs_to'
	`, orgID, objectID, from, to, targetID)
	if err != nil {
		if isUnique(err) {
			return apiErr(http.StatusConflict, "relationship already exists")
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return insertBelongsTo(ctx, tx, orgID, objectID, targetID, to)
	}
	return nil
}

func renameInverse(ctx context.Context, tx pgx.Tx, orgID, objectID, from, to string) error {
	_, err := tx.Exec(ctx, `
		update relationship_definitions
		set inverse_name = $4
		where organization_id = $1
		  and to_object_id = $2
		  and inverse_name = $3
		  and kind = 'has_many'
	`, orgID, objectID, from, to)
	return err
}

func deleteInverseLists(ctx context.Context, tx pgx.Tx, orgID, objectID, field string) error {
	if _, err := tx.Exec(ctx, `
		delete from record_links
		where relationship_id in (
			select id from relationship_definitions
			where organization_id = $1
			  and to_object_id = $2
			  and inverse_name = $3
			  and kind = 'has_many'
		)
	`, orgID, objectID, field); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		delete from relationship_definitions
		where organization_id = $1
		  and to_object_id = $2
		  and inverse_name = $3
		  and kind = 'has_many'
	`, orgID, objectID, field)
	return err
}

func deleteFieldRelationships(ctx context.Context, tx pgx.Tx, orgID, objectID, field string) error {
	if _, err := tx.Exec(ctx, `
		delete from record_links
		where relationship_id in (
			select id from relationship_definitions
			where organization_id = $1
			  and (
				(from_object_id = $2 and name = $3 and kind = 'belongs_to')
				or (to_object_id = $2 and inverse_name = $3 and kind = 'has_many')
			  )
		)
	`, orgID, objectID, field); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		delete from relationship_definitions
		where organization_id = $1
		  and (
			(from_object_id = $2 and name = $3 and kind = 'belongs_to')
			or (to_object_id = $2 and inverse_name = $3 and kind = 'has_many')
		  )
	`, orgID, objectID, field)
	return err
}

func moveFieldValues(ctx context.Context, tx pgx.Tx, orgID, objectID, from, to string) error {
	_, err := tx.Exec(ctx, `
		update records
		set data = (data - $1::text) || jsonb_build_object($2::text, data -> $1::text),
			version = version + 1,
			updated_at = now()
		where organization_id = $3::uuid
		  and object_id = $4::uuid
		  and jsonb_exists(data, $1::text)
	`, from, to, orgID, objectID)
	return err
}

func clearFieldValues(ctx context.Context, tx pgx.Tx, orgID, objectID, name string) error {
	_, err := tx.Exec(ctx, `
		update records
		set data = data - $1::text,
			version = version + 1,
			updated_at = now()
		where organization_id = $2::uuid
		  and object_id = $3::uuid
		  and jsonb_exists(data, $1::text)
	`, name, orgID, objectID)
	return err
}
