package main

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type inboundInput struct {
	Name   string `json:"name"`
	Object string `json:"object"`
}

type inboundView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Object string `json:"object"`
	Secret string `json:"secret,omitempty"`
}

func (s *server) createInbound(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body inboundInput
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if !namePattern.MatchString(body.Name) {
		writeErr(w, apiErr(http.StatusBadRequest, "invalid name"))
		return
	}
	objectID, err := s.objectByName(r.Context(), a.OrgID, body.Object)
	if err != nil {
		writeErr(w, err)
		return
	}
	secret, err := newSecret()
	if err != nil {
		writeErr(w, err)
		return
	}
	view := inboundView{ID: uuid.NewString(), Name: body.Name, Object: body.Object, Secret: secret}
	_, err = s.pool.Exec(r.Context(), `
		insert into inbound_endpoints (id, organization_id, name, object_id, secret, actor_id)
		values ($1, $2, $3, $4, $5, $6)
	`, view.ID, a.OrgID, view.Name, objectID, secret, a.UserID)
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "inbound already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listInbound(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		select e.id::text, e.name, o.name
		from inbound_endpoints e
		join object_definitions o on o.id = e.object_id
		where e.organization_id = $1::uuid
		order by e.name
	`, a.OrgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	hooks := []inboundView{}
	for rows.Next() {
		var view inboundView
		if err := rows.Scan(&view.ID, &view.Name, &view.Object); err != nil {
			writeErr(w, err)
			return
		}
		hooks = append(hooks, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"inbound": hooks})
}

func (s *server) deleteInbound(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		delete from inbound_endpoints
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

// acceptInbound verifies the signature, then stores a record. The workflow runs later, from the event.
func (s *server) acceptInbound(w http.ResponseWriter, r *http.Request) {
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	var orgID, objectID, objectName, secret, actorID string
	err := s.pool.QueryRow(r.Context(), `
		select e.organization_id::text, e.object_id::text, o.name, e.secret, e.actor_id::text
		from inbound_endpoints e
		join object_definitions o on o.id = e.object_id
		where e.id = $1::uuid
	`, r.PathValue("id")).Scan(&orgID, &objectID, &objectName, &secret, &actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !hmac.Equal([]byte(signWebhook(secret, raw)), []byte(r.Header.Get("X-Openforce-Signature"))) {
		writeErr(w, apiErr(http.StatusUnauthorized, "unauthorized"))
		return
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		writeErr(w, apiErr(http.StatusBadRequest, "invalid json"))
		return
	}
	actor, err := s.memberActor(r.Context(), orgID, actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, apiErr(http.StatusUnauthorized, "unauthorized"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.authorize(r.Context(), actor, objectID, true); err != nil {
		writeErr(w, err)
		return
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
	if err := s.checkRelations(r.Context(), orgID, objectID, defs, data); err != nil {
		writeErr(w, err)
		return
	}
	view, err := s.insertRecord(r.Context(), actor, objectID, objectName, defs, data, sourceIntegration, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}
