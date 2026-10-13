package main

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type relationshipBody struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	To      string `json:"to"`
	Inverse string `json:"inverse"`
}

type relationship struct {
	ID      string
	FromID  string
	ToID    string
	Name    string
	Kind    string
	Inverse string
	Forward bool
}

type linkBody struct {
	ID string `json:"id"`
}

func (s *server) createRelationship(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if a.Role != "owner" {
		writeErr(w, apiErr(http.StatusForbidden, "forbidden"))
		return
	}
	fromID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body relationshipBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if !namePattern.MatchString(body.Name) || !namePattern.MatchString(body.To) {
		writeErr(w, apiErr(http.StatusBadRequest, "invalid name"))
		return
	}
	toID, err := s.objectByName(r.Context(), a.OrgID, body.To)
	if err != nil {
		writeErr(w, err)
		return
	}
	switch body.Kind {
	case "belongs_to", "has_many", "many_to_many":
	default:
		writeErr(w, apiErr(http.StatusBadRequest, "invalid relationship"))
		return
	}
	if body.Kind != "belongs_to" && !namePattern.MatchString(body.Inverse) {
		writeErr(w, apiErr(http.StatusBadRequest, "inverse is required"))
		return
	}
	if err := s.insertRelationship(r.Context(), a.OrgID, fromID, toID, body); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"name": body.Name,
		"kind": body.Kind,
		"to":   body.To,
	})
}

func (s *server) insertRelationship(ctx context.Context, orgID, fromID, toID string, body relationshipBody) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if body.Kind == "has_many" {
		var exists bool
		err = tx.QueryRow(ctx, `
			select exists (
				select 1 from relationship_definitions
				where organization_id = $1
				  and from_object_id = $2
				  and to_object_id = $3
				  and name = $4
				  and kind = 'belongs_to'
			)
		`, orgID, toID, fromID, body.Inverse).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return apiErr(http.StatusBadRequest, "inverse belongs_to was not found")
		}
	}

	_, err = tx.Exec(ctx, `
		insert into relationship_definitions
			(id, organization_id, from_object_id, to_object_id, name, kind, inverse_name)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), orgID, fromID, toID, body.Name, body.Kind, emptyAsNil(body.Inverse))
	if err != nil {
		if isUnique(err) {
			return apiErr(http.StatusConflict, "relationship already exists")
		}
		return err
	}
	if body.Kind == "belongs_to" {
		var position int
		if err := tx.QueryRow(ctx, `select coalesce(max(position) + 1, 0) from field_definitions where object_id = $1`, fromID).Scan(&position); err != nil {
			return err
		}
		if err := insertField(ctx, tx, orgID, fromID, body.Name, "relation", false, nil, position); err != nil {
			if isUnique(err) {
				return apiErr(http.StatusConflict, "field already exists")
			}
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *server) listRelated(w http.ResponseWriter, r *http.Request) {
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
	rel, err := s.findRelationship(r.Context(), a.OrgID, objectID, r.PathValue("rel"))
	if err != nil {
		writeErr(w, err)
		return
	}
	otherID := rel.ToID
	if !rel.Forward {
		otherID = rel.FromID
	}
	if err := s.authorize(r.Context(), a, otherID, false); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.selectRecord(r.Context(), a.OrgID, objectID, r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	views, err := s.relatedRecords(r.Context(), a.OrgID, rel, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": views})
}

func (s *server) linkRelated(w http.ResponseWriter, r *http.Request) {
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
	rel, err := s.findRelationship(r.Context(), a.OrgID, objectID, r.PathValue("rel"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !rel.Forward || rel.Kind != "many_to_many" {
		writeErr(w, apiErr(http.StatusBadRequest, "only many_to_many links can be created here"))
		return
	}
	if err := s.authorize(r.Context(), a, rel.ToID, false); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.selectRecord(r.Context(), a.OrgID, objectID, r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	var body linkBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.selectRecord(r.Context(), a.OrgID, rel.ToID, body.ID); err != nil {
		writeErr(w, err)
		return
	}
	_, err = s.pool.Exec(r.Context(), `
		insert into record_links (id, organization_id, relationship_id, from_record_id, to_record_id)
		values ($1, $2, $3, $4, $5)
	`, uuid.NewString(), a.OrgID, rel.ID, r.PathValue("id"), body.ID)
	if err != nil {
		if isUnique(err) {
			writeErr(w, apiErr(http.StatusConflict, "link already exists"))
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": body.ID})
}

func (s *server) findRelationship(ctx context.Context, orgID, objectID, name string) (relationship, error) {
	var rel relationship
	var inverse *string
	err := s.pool.QueryRow(ctx, `
		select id::text, from_object_id::text, to_object_id::text, name, kind, inverse_name
		from relationship_definitions
		where organization_id = $1 and from_object_id = $2 and name = $3
	`, orgID, objectID, name).Scan(&rel.ID, &rel.FromID, &rel.ToID, &rel.Name, &rel.Kind, &inverse)
	if err == nil {
		rel.Forward = true
		if inverse != nil {
			rel.Inverse = *inverse
		}
		return rel, nil
	}
	if err != pgx.ErrNoRows {
		return relationship{}, err
	}
	err = s.pool.QueryRow(ctx, `
		select id::text, from_object_id::text, to_object_id::text, name, kind, inverse_name
		from relationship_definitions
		where organization_id = $1 and to_object_id = $2 and inverse_name = $3
	`, orgID, objectID, name).Scan(&rel.ID, &rel.FromID, &rel.ToID, &rel.Name, &rel.Kind, &inverse)
	if err == pgx.ErrNoRows {
		return relationship{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return relationship{}, err
	}
	if inverse != nil {
		rel.Inverse = *inverse
	}
	return rel, nil
}

func (s *server) relatedRecords(ctx context.Context, orgID string, rel relationship, recordID string) ([]recordView, error) {
	switch rel.Kind {
	case "belongs_to":
		if !rel.Forward {
			return s.recordsPointingAt(ctx, orgID, rel.FromID, rel.Name, recordID)
		}
		view, err := s.selectRecord(ctx, orgID, rel.FromID, recordID)
		if err != nil {
			return nil, err
		}
		raw, _ := view.Data[rel.Name].(string)
		if raw == "" {
			return []recordView{}, nil
		}
		target, err := s.selectRecord(ctx, orgID, rel.ToID, raw)
		if err != nil {
			return []recordView{}, nil
		}
		return []recordView{target}, nil
	case "has_many":
		return s.recordsPointingAt(ctx, orgID, rel.ToID, rel.Inverse, recordID)
	case "many_to_many":
		column := "from_record_id"
		other := "to_record_id"
		objectID := rel.ToID
		if !rel.Forward {
			column = "to_record_id"
			other = "from_record_id"
			objectID = rel.FromID
		}
		rows, err := s.pool.Query(ctx, `
			select r.id::text, r.version, r.data
			from record_links l
			join records r on r.id = l.`+other+`
			where l.relationship_id = $1 and l.organization_id = $2
			  and l.`+column+` = $3::uuid
			  and r.organization_id = l.organization_id
			  and r.deleted_at is null and r.object_id = $4
			order by r.created_at, r.id
		`, rel.ID, orgID, recordID, objectID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanRecords(rows)
	default:
		return nil, apiErr(http.StatusBadRequest, "invalid relationship")
	}
}

func (s *server) recordsPointingAt(ctx context.Context, orgID, objectID, field, recordID string) ([]recordView, error) {
	rows, err := s.pool.Query(ctx, `
		select id::text, version, data
		from records
		where organization_id = $1 and object_id = $2 and deleted_at is null
		  and data->>$3 = $4
		order by created_at, id
	`, orgID, objectID, field, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func scanRecords(rows pgx.Rows) ([]recordView, error) {
	views := []recordView{}
	for rows.Next() {
		var view recordView
		var raw []byte
		if err := rows.Scan(&view.ID, &view.Version, &raw); err != nil {
			return nil, err
		}
		var err error
		view.Data, err = decodeData(raw)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (s *server) checkRelations(ctx context.Context, orgID, objectID string, defs []fieldDef, data map[string]any) error {
	for _, def := range defs {
		if def.Type != "relation" {
			continue
		}
		value, ok := data[def.Name]
		if !ok || value == nil || value == "" {
			continue
		}
		id, ok := value.(string)
		if !ok {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		if _, err := uuid.Parse(id); err != nil {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		var target string
		err := s.pool.QueryRow(ctx, `
			select to_object_id::text
			from relationship_definitions
			where organization_id = $1 and from_object_id = $2 and name = $3 and kind = 'belongs_to'
		`, orgID, objectID, def.Name).Scan(&target)
		if err == pgx.ErrNoRows {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		if err != nil {
			return err
		}
		var found bool
		err = s.pool.QueryRow(ctx, `
			select exists (
				select 1 from records
				where id = $1::uuid and organization_id = $2::uuid and object_id = $3::uuid and deleted_at is null
			)
		`, id, orgID, target).Scan(&found)
		if err != nil {
			return err
		}
		if !found {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
	}
	return nil
}

func emptyAsNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
