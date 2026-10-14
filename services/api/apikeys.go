package main

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type apiKeyGrant struct {
	Object   string `json:"object"`
	CanRead  bool   `json:"can_read"`
	CanWrite bool   `json:"can_write"`
}

type apiKeyInput struct {
	Name   string        `json:"name"`
	Label  string        `json:"label"`
	Grants []apiKeyGrant `json:"grants"`
}

type apiKeyPatch struct {
	Grants []apiKeyGrant `json:"grants"`
}

type apiKeyView struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Label  string        `json:"label"`
	Grants []apiKeyGrant `json:"grants"`
	Token  string        `json:"token,omitempty"`
}

type resolvedGrant struct {
	ObjectID string
	Grant    apiKeyGrant
}

func (s *server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body apiKeyInput
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if !namePattern.MatchString(body.Name) {
		writeErr(w, apiErr(http.StatusBadRequest, "invalid name"))
		return
	}
	if body.Label == "" {
		writeErr(w, apiErr(http.StatusBadRequest, "label is required"))
		return
	}
	grants, err := s.resolveGrants(r.Context(), a.OrgID, body.Grants)
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, hash, err := newToken()
	if err != nil {
		writeErr(w, err)
		return
	}
	userID := uuid.NewString()
	keyID := uuid.NewString()
	email := strings.ToLower(userID + "@keys.openforce.invalid")
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `insert into users (id, email, name) values ($1, $2, $3)`, userID, email, body.Label); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		insert into memberships (organization_id, user_id, role) values ($1, $2, 'service')
	`, a.OrgID, userID); err != nil {
		writeErr(w, err)
		return
	}
	_, err = tx.Exec(r.Context(), `
		insert into api_keys (id, organization_id, user_id, name, label, token_hash)
		values ($1, $2, $3, $4, $5, $6)
	`, keyID, a.OrgID, userID, body.Name, body.Label, hash)
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "api key already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = insertKeyGrants(r.Context(), tx, a.OrgID, keyID, grants); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = insertEvent(r.Context(), tx, a.OrgID, eventWrite{
		Name:    "user.created",
		ActorID: a.UserID,
		Source:  sourceAdmin,
		Payload: map[string]any{"user_id": userID, "email": email, "name": body.Label},
	}); err != nil {
		writeErr(w, err)
		return
	}
	if err = s.committed(r.Context(), tx); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.loadAPIKey(r.Context(), a.OrgID, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	view.Token = raw
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	keys, err := s.loadAPIKeys(r.Context(), a.OrgID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s *server) getAPIKey(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	view, err := s.loadAPIKey(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) patchAPIKey(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	current, err := s.loadAPIKey(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body apiKeyPatch
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	grants, err := s.resolveGrants(r.Context(), a.OrgID, body.Grants)
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
	if _, err = tx.Exec(r.Context(), `delete from api_key_permissions where api_key_id = $1::uuid`, current.ID); err != nil {
		writeErr(w, err)
		return
	}
	if err = insertKeyGrants(r.Context(), tx, a.OrgID, current.ID, grants); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.loadAPIKey(r.Context(), a.OrgID, current.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		delete from api_keys
		where organization_id = $1::uuid and name = $2
	`, a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) resolveGrants(ctx context.Context, orgID string, grants []apiKeyGrant) ([]resolvedGrant, error) {
	if grants == nil {
		return nil, apiErr(http.StatusBadRequest, "grants are required")
	}
	seen := map[string]struct{}{}
	resolved := make([]resolvedGrant, 0, len(grants))
	for _, grant := range grants {
		if !namePattern.MatchString(grant.Object) {
			return nil, apiErr(http.StatusBadRequest, "invalid name")
		}
		if _, ok := seen[grant.Object]; ok {
			return nil, apiErr(http.StatusBadRequest, "duplicate object")
		}
		seen[grant.Object] = struct{}{}
		objectID, err := s.objectByName(ctx, orgID, grant.Object)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, resolvedGrant{ObjectID: objectID, Grant: grant})
	}
	return resolved, nil
}

func insertKeyGrants(ctx context.Context, tx pgx.Tx, orgID, keyID string, grants []resolvedGrant) error {
	for _, grant := range grants {
		if _, err := tx.Exec(ctx, `
			insert into api_key_permissions (api_key_id, organization_id, object_id, can_read, can_write)
			values ($1, $2, $3, $4, $5)
		`, keyID, orgID, grant.ObjectID, grant.Grant.CanRead, grant.Grant.CanWrite); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) loadAPIKey(ctx context.Context, orgID, name string) (apiKeyView, error) {
	keys, err := s.loadAPIKeys(ctx, orgID, name)
	if err != nil {
		return apiKeyView{}, err
	}
	if len(keys) != 1 {
		return apiKeyView{}, apiErr(http.StatusNotFound, "not found")
	}
	return keys[0], nil
}

func (s *server) loadAPIKeys(ctx context.Context, orgID, name string) ([]apiKeyView, error) {
	args := []any{orgID}
	query := `
		select k.id::text, k.name, k.label, o.name, p.can_read, p.can_write
		from api_keys k
		left join api_key_permissions p on p.api_key_id = k.id
		left join object_definitions o on o.id = p.object_id
		where k.organization_id = $1::uuid`
	if name != "" {
		args = append(args, name)
		query += ` and k.name = $2`
	}
	query += ` order by k.name, o.name`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	order := []string{}
	byID := map[string]*apiKeyView{}
	for rows.Next() {
		var id, keyName, label string
		var object sql.NullString
		var canRead, canWrite sql.NullBool
		if err := rows.Scan(&id, &keyName, &label, &object, &canRead, &canWrite); err != nil {
			return nil, err
		}
		view, ok := byID[id]
		if !ok {
			view = &apiKeyView{ID: id, Name: keyName, Label: label, Grants: []apiKeyGrant{}}
			byID[id] = view
			order = append(order, id)
		}
		if object.Valid {
			view.Grants = append(view.Grants, apiKeyGrant{
				Object: object.String, CanRead: canRead.Bool, CanWrite: canWrite.Bool,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := make([]apiKeyView, 0, len(order))
	for _, id := range order {
		keys = append(keys, *byID[id])
	}
	return keys, nil
}

func (s *server) authorizeKey(ctx context.Context, a actor, objectID string, write bool) error {
	if a.KeyID == "" {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	var canRead, canWrite bool
	err := s.pool.QueryRow(ctx, `
		select can_read, can_write
		from api_key_permissions
		where api_key_id = $1 and object_id = $2
	`, a.KeyID, objectID).Scan(&canRead, &canWrite)
	if err == pgx.ErrNoRows {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	if err != nil {
		return err
	}
	if write && !canWrite {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	if !write && !canRead {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	return nil
}
