package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

// functionHandlers are Go functions compiled into this process.
// A missing or nil entry fails the job. Callers do not upload source.
var (
	functionMu       sync.RWMutex
	functionHandlers = map[string]functionHandler{}
)

type functionHandler func(ctx context.Context, event functionEvent) error

type functionEvent struct {
	Name     string
	Object   string
	RecordID string
	ActorID  string
	Data     map[string]any

	ctx     context.Context
	server  *server
	actor   actor
	cause   string
	created *[]string
}

type functionInput struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type functionView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

func registerFunction(name string, handler functionHandler) {
	functionMu.Lock()
	defer functionMu.Unlock()
	functionHandlers[name] = handler
}

func clearFunction(name string) {
	functionMu.Lock()
	defer functionMu.Unlock()
	delete(functionHandlers, name)
}

func lookupFunction(name string) functionHandler {
	functionMu.RLock()
	defer functionMu.RUnlock()
	return functionHandlers[name]
}

func (s *server) createFunction(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body functionInput
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
	view := functionView{ID: uuid.NewString(), Name: body.Name, Label: body.Label}
	_, err := s.pool.Exec(r.Context(), `
		insert into functions (id, organization_id, name, label)
		values ($1, $2, $3, $4)
	`, view.ID, a.OrgID, view.Name, view.Label)
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "function already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listFunctions(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		select id::text, name, label
		from functions
		where organization_id = $1::uuid
		order by name
	`, a.OrgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	views := []functionView{}
	for rows.Next() {
		var view functionView
		if err := rows.Scan(&view.ID, &view.Name, &view.Label); err != nil {
			writeErr(w, err)
			return
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"functions": views})
}

func (s *server) deleteFunction(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		delete from functions
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

func (s *server) validateFunctionAction(ctx context.Context, orgID string, action automationAction) error {
	if action.Object != "" || action.URL != "" || action.Record != nil || len(action.Data) > 0 {
		return apiErr(http.StatusBadRequest, "invalid action")
	}
	if !namePattern.MatchString(action.Name) {
		return apiErr(http.StatusBadRequest, "invalid name")
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		select exists (
			select 1 from functions
			where organization_id = $1::uuid and name = $2
		)
	`, orgID, action.Name).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return apiErr(http.StatusBadRequest, "not found")
	}
	return nil
}

func (s *server) runFunction(ctx context.Context, a actor, eventName, sourceName, recordID string, record map[string]any, action automationAction, cause string) ([]string, error) {
	handler := lookupFunction(action.Name)
	if handler == nil {
		return nil, fmt.Errorf("unknown function")
	}
	if record == nil {
		record = map[string]any{}
	}
	created := []string{}
	event := functionEvent{
		Name: eventName, Object: sourceName, RecordID: recordID, ActorID: a.UserID, Data: record,
		ctx: ctx, server: s, actor: a, cause: cause, created: &created,
	}
	if err := handler(ctx, event); err != nil {
		return nil, err
	}
	return created, nil
}

// Create writes a record as the event actor. Policy applies, and the id is kept for the execution.
func (e functionEvent) Create(object string, data map[string]any) (string, error) {
	objectID, err := e.server.objectByName(e.ctx, e.actor.OrgID, object)
	if err != nil {
		return "", err
	}
	if err := e.server.authorize(e.ctx, e.actor, objectID, true); err != nil {
		return "", err
	}
	defs, err := e.server.fieldDefs(e.ctx, objectID)
	if err != nil {
		return "", err
	}
	if err := validateData(defs, data, true); err != nil {
		return "", err
	}
	if err := e.server.checkRelations(e.ctx, e.actor.OrgID, objectID, defs, data); err != nil {
		return "", err
	}
	view, err := e.server.insertRecord(e.ctx, e.actor, objectID, object, defs, data, sourceAutomation, e.cause)
	if err != nil {
		return "", err
	}
	if e.created != nil {
		*e.created = append(*e.created, view.ID)
	}
	return view.ID, nil
}

// Update patches a record as the event actor. Policy applies. An update is not a created id.
func (e functionEvent) Update(object, id string, patch map[string]any) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("invalid record")
	}
	objectID, err := e.server.objectByName(e.ctx, e.actor.OrgID, object)
	if err != nil {
		return err
	}
	if err := e.server.authorize(e.ctx, e.actor, objectID, true); err != nil {
		return err
	}
	_, err = e.server.writeRecordPatch(e.ctx, e.actor, objectID, object, id, patch, sourceAutomation, e.cause)
	return err
}
