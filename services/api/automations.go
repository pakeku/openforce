package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type automationAction struct {
	Type   string         `json:"type"`
	Object string         `json:"object,omitempty"`
	Record any            `json:"record,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
	URL    string         `json:"url,omitempty"`
}

type automationInput struct {
	Name       string             `json:"name"`
	Label      string             `json:"label"`
	Event      string             `json:"event"`
	Object     string             `json:"object"`
	Every      string             `json:"every"`
	Conditions []condition        `json:"conditions"`
	Actions    []automationAction `json:"actions"`
}

type automationPlan struct {
	ObjectID string
	Every    int
}

type automationPatch struct {
	Version    int                 `json:"version"`
	Label      *string             `json:"label"`
	Conditions *[]condition        `json:"conditions"`
	Actions    *[]automationAction `json:"actions"`
}

type automationView struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Label      string             `json:"label"`
	Event      string             `json:"event,omitempty"`
	Object     string             `json:"object,omitempty"`
	ObjectID   string             `json:"-"`
	Every      string             `json:"every,omitempty"`
	NextRun    *time.Time         `json:"next_run_at,omitempty"`
	Version    int                `json:"version"`
	Conditions []condition        `json:"conditions"`
	Actions    []automationAction `json:"actions"`
}

type executionView struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	Status    string    `json:"status"`
	Created   []string  `json:"created"`
	CreatedAt time.Time `json:"created_at"`
}

// enqueueAutomationJobs stores the work. The request does not run the workflow.
func enqueueAutomationJobs(ctx context.Context, tx pgx.Tx, orgID, eventID, name, objectID, excludeID string) error {
	if objectID == "" {
		return nil
	}
	args := []any{orgID, name, objectID}
	query := `
		select id::text
		from automation_definitions
		where organization_id = $1::uuid
			and event_name = $2
			and object_id = $3::uuid`
	if excludeID != "" {
		args = append(args, excludeID)
		query += " and id <> $4::uuid"
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := enqueueJob(ctx, tx, orgID, eventID, "automation", "", id); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) createAutomation(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body automationInput
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	plan, err := s.prepareAutomation(r.Context(), a.OrgID, &body)
	if err != nil {
		writeErr(w, err)
		return
	}
	conditions, actions, err := encodeAutomation(body.Conditions, body.Actions)
	if err != nil {
		writeErr(w, err)
		return
	}
	view := automationView{
		ID: uuid.NewString(), Name: body.Name, Label: body.Label, Event: body.Event,
		Object: body.Object, ObjectID: plan.ObjectID, Version: 1,
		Conditions: body.Conditions, Actions: body.Actions,
	}
	var eventName, every, nextAt, actor any
	if view.Event != "" {
		eventName = view.Event
	}
	if plan.Every > 0 {
		when := time.Now().UTC().Add(time.Duration(plan.Every) * time.Second)
		every = plan.Every
		nextAt = when
		actor = a.UserID
		view.Every = formatEvery(plan.Every)
		view.NextRun = &when
	}
	_, err = s.pool.Exec(r.Context(), `
		insert into automation_definitions
			(id, organization_id, name, label, event_name, object_id, conditions, actions, every_seconds, next_run_at, actor_id)
		values ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10, $11)
	`, view.ID, a.OrgID, view.Name, view.Label, eventName, uuidOrNil(plan.ObjectID), conditions, actions, every, nextAt, actor)
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "automation already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listAutomations(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	views, err := s.selectAutomations(r.Context(), a.OrgID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"automations": views})
}

func (s *server) getAutomation(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	view, err := s.loadAutomationByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) patchAutomation(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body automationPatch
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
	current, err := lockAutomation(r.Context(), tx, a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if current.Version != body.Version {
		writeErr(w, apiErr(http.StatusConflict, "stale version"))
		return
	}
	next := automationInput{
		Name: current.Name, Label: current.Label, Event: current.Event, Object: current.Object,
		Every: current.Every, Conditions: current.Conditions, Actions: current.Actions,
	}
	if body.Label != nil {
		next.Label = *body.Label
	}
	if body.Conditions != nil {
		next.Conditions = *body.Conditions
	}
	if body.Actions != nil {
		next.Actions = *body.Actions
	}
	if _, err := s.prepareAutomation(r.Context(), a.OrgID, &next); err != nil {
		writeErr(w, err)
		return
	}
	conditions, actions, err := encodeAutomation(next.Conditions, next.Actions)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		update automation_definitions
		set label = $1, conditions = $2::jsonb, actions = $3::jsonb,
			version = version + 1, updated_at = now()
		where id = $4::uuid and version = $5
		returning version
	`, next.Label, conditions, actions, current.ID, body.Version).Scan(&current.Version); err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	current.Label = next.Label
	current.Conditions = next.Conditions
	current.Actions = next.Actions
	writeJSON(w, http.StatusOK, current)
}

func (s *server) deleteAutomation(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		delete from automation_definitions
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

func (s *server) listExecutions(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	view, err := s.loadAutomationByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		select id::text, event_id::text, status, created_ids, created_at
		from automation_executions
		where organization_id = $1::uuid and automation_id = $2::uuid
		order by created_at, id
	`, a.OrgID, view.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	executions := []executionView{}
	for rows.Next() {
		var view executionView
		var raw []byte
		if err := rows.Scan(&view.ID, &view.EventID, &view.Status, &raw, &view.CreatedAt); err != nil {
			writeErr(w, err)
			return
		}
		view.Created = []string{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &view.Created); err != nil {
				writeErr(w, err)
				return
			}
		}
		if view.Created == nil {
			view.Created = []string{}
		}
		executions = append(executions, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executions": executions})
}

func (s *server) prepareAutomation(ctx context.Context, orgID string, body *automationInput) (automationPlan, error) {
	if !namePattern.MatchString(body.Name) {
		return automationPlan{}, apiErr(http.StatusBadRequest, "invalid name")
	}
	if body.Label == "" {
		return automationPlan{}, apiErr(http.StatusBadRequest, "label is required")
	}
	every, err := parseEvery(body.Every)
	if err != nil {
		return automationPlan{}, err
	}
	if body.Conditions == nil {
		body.Conditions = []condition{}
	}
	if every > 0 {
		if body.Event != "" || body.Object != "" || len(body.Conditions) > 0 {
			return automationPlan{}, apiErr(http.StatusBadRequest, "invalid schedule")
		}
		if err := s.validateActions(ctx, orgID, "", map[string]fieldDef{}, body.Actions); err != nil {
			return automationPlan{}, err
		}
		return automationPlan{Every: every}, nil
	}
	switch body.Event {
	case "record.created", "record.updated", "record.deleted":
	default:
		return automationPlan{}, apiErr(http.StatusBadRequest, "invalid event")
	}
	if !namePattern.MatchString(body.Object) {
		return automationPlan{}, apiErr(http.StatusBadRequest, "invalid name")
	}
	objectID, err := s.objectByName(ctx, orgID, body.Object)
	if err != nil {
		return automationPlan{}, err
	}
	defs, err := s.fieldDefs(ctx, objectID)
	if err != nil {
		return automationPlan{}, err
	}
	byName := indexFields(defs)
	for _, cond := range body.Conditions {
		def, ok := byName[cond.Field]
		if !ok {
			return automationPlan{}, apiErrf(http.StatusBadRequest, "unknown field %s", cond.Field)
		}
		if err := validateCondition(def, cond); err != nil {
			return automationPlan{}, err
		}
	}
	if err := s.validateActions(ctx, orgID, body.Object, byName, body.Actions); err != nil {
		return automationPlan{}, err
	}
	return automationPlan{ObjectID: objectID}, nil
}

func parseEvery(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	if len(raw) < 2 {
		return 0, apiErr(http.StatusBadRequest, "invalid schedule")
	}
	unit := raw[len(raw)-1]
	num := raw[:len(raw)-1]
	n, err := strconv.Atoi(num)
	if err != nil || strconv.Itoa(n) != num || n < 1 {
		return 0, apiErr(http.StatusBadRequest, "invalid schedule")
	}
	var seconds int64
	switch unit {
	case 'm':
		seconds = int64(n) * 60
	case 'h':
		seconds = int64(n) * 3600
	case 'd':
		seconds = int64(n) * 86400
	default:
		return 0, apiErr(http.StatusBadRequest, "invalid schedule")
	}
	if seconds < 60 || seconds > 366*86400 {
		return 0, apiErr(http.StatusBadRequest, "invalid schedule")
	}
	return int(seconds), nil
}

func formatEvery(seconds int) string {
	if seconds >= 86400 && seconds%86400 == 0 {
		return strconv.Itoa(seconds/86400) + "d"
	}
	if seconds >= 3600 && seconds%3600 == 0 {
		return strconv.Itoa(seconds/3600) + "h"
	}
	return strconv.Itoa(seconds/60) + "m"
}

func validateCondition(def fieldDef, cond condition) error {
	if !conditionOp(def.Type, cond.Op) {
		return apiErrf(http.StatusBadRequest, "invalid operator for %s", cond.Field)
	}
	if cond.Op == "contains" {
		if _, ok := cond.Value.(string); !ok {
			return apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		return nil
	}
	return validateValue(def, cond.Value)
}

func conditionOp(fieldType, op string) bool {
	switch op {
	case "eq", "neq":
		return true
	case "contains":
		return fieldType == "text"
	case "gt", "gte", "lt", "lte":
		return fieldType == "number" || fieldType == "currency" || fieldType == "date"
	default:
		return false
	}
}

func (s *server) validateActions(ctx context.Context, orgID, sourceName string, source map[string]fieldDef, actions []automationAction) error {
	if len(actions) == 0 {
		return apiErr(http.StatusBadRequest, "actions are required")
	}
	for _, action := range actions {
		switch action.Type {
		case "http":
			if err := validateHTTPAction(sourceName, source, action); err != nil {
				return err
			}
			continue
		case "create", "update", "delete":
			if sourceName == "" && action.Type != "create" {
				return apiErr(http.StatusBadRequest, "invalid action")
			}
		default:
			return apiErr(http.StatusBadRequest, "invalid action")
		}
		if err := validateActionRecord(action.Record); err != nil {
			return err
		}
		if action.Type == "delete" && len(action.Data) > 0 {
			return apiErr(http.StatusBadRequest, "invalid action")
		}
		if action.Type == "update" && len(action.Data) == 0 {
			return apiErr(http.StatusBadRequest, "data is required")
		}
		defs, err := s.actionFields(ctx, orgID, sourceName, source, action)
		if err != nil {
			return err
		}
		if action.Type == "delete" {
			continue
		}
		sample, err := actionSample(sourceName, source, action.Data)
		if err != nil {
			return err
		}
		if err := validateData(defs, sample, action.Type == "create"); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) actionFields(ctx context.Context, orgID, sourceName string, source map[string]fieldDef, action automationAction) ([]fieldDef, error) {
	if action.Type != "create" && action.Object == "" {
		defs := make([]fieldDef, 0, len(source))
		for _, def := range source {
			defs = append(defs, def)
		}
		return defs, nil
	}
	if !namePattern.MatchString(action.Object) {
		return nil, apiErr(http.StatusBadRequest, "invalid name")
	}
	if action.Type != "create" && action.Object != sourceName && action.Record == nil {
		return nil, apiErr(http.StatusBadRequest, "invalid record")
	}
	targetID, err := s.objectByName(ctx, orgID, action.Object)
	if err != nil {
		return nil, err
	}
	return s.fieldDefs(ctx, targetID)
}

func actionSample(sourceName string, source map[string]fieldDef, data map[string]any) (map[string]any, error) {
	sample := map[string]any{}
	for key, value := range data {
		if field, ok := fieldRef(value); ok {
			sourceDef, known := source[field]
			if !known {
				return nil, apiErrf(http.StatusBadRequest, "unknown field %s", field)
			}
			copied, err := sampleValue(sourceDef)
			if err != nil {
				return nil, err
			}
			sample[key] = copied
			continue
		}
		if isRecordRef(value) {
			if sourceName == "" {
				return nil, apiErr(http.StatusBadRequest, "invalid record")
			}
			sample[key] = uuid.NewString()
			continue
		}
		sample[key] = value
	}
	return sample, nil
}

func validateActionRecord(value any) error {
	if value == nil || isRecordRef(value) {
		return nil
	}
	if _, ok := fieldRef(value); ok {
		return nil
	}
	return apiErr(http.StatusBadRequest, "invalid record")
}

func sampleValue(def fieldDef) (any, error) {
	switch def.Type {
	case "text", "relation":
		return "sample", nil
	case "number", "currency":
		return float64(1), nil
	case "boolean":
		return true, nil
	case "date":
		return "2020-01-01", nil
	case "enum":
		if len(def.Options) == 0 {
			return nil, apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
		}
		return def.Options[0], nil
	default:
		return nil, apiErrf(http.StatusBadRequest, "invalid value for %s", def.Name)
	}
}

func fieldRef(value any) (string, bool) {
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 1 {
		return "", false
	}
	name, ok := fields["field"].(string)
	return name, ok && namePattern.MatchString(name)
}

func isRecordRef(value any) bool {
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 1 {
		return false
	}
	flag, ok := fields["record"].(bool)
	return ok && flag
}

func encodeAutomation(conditions []condition, actions []automationAction) (string, string, error) {
	if conditions == nil {
		conditions = []condition{}
	}
	rawConditions, err := json.Marshal(conditions)
	if err != nil {
		return "", "", err
	}
	rawActions, err := json.Marshal(actions)
	if err != nil {
		return "", "", err
	}
	return string(rawConditions), string(rawActions), nil
}

func (s *server) selectAutomations(ctx context.Context, orgID, name string) ([]automationView, error) {
	args := []any{orgID}
	query := `
		select d.id::text, d.name, d.label, d.event_name, o.name, d.object_id::text,
			d.version, d.conditions, d.actions, d.every_seconds, d.next_run_at
		from automation_definitions d
		left join object_definitions o on o.id = d.object_id
		where d.organization_id = $1::uuid`
	if name != "" {
		args = append(args, name)
		query += fmt.Sprintf(" and d.name = $%d", len(args))
	}
	query += " order by d.name"
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []automationView{}
	for rows.Next() {
		view, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (s *server) loadAutomationByName(ctx context.Context, orgID, name string) (automationView, error) {
	views, err := s.selectAutomations(ctx, orgID, name)
	if err != nil {
		return automationView{}, err
	}
	if len(views) == 0 {
		return automationView{}, apiErr(http.StatusNotFound, "not found")
	}
	return views[0], nil
}

func lockAutomation(ctx context.Context, tx pgx.Tx, orgID, name string) (automationView, error) {
	row := tx.QueryRow(ctx, `
		select d.id::text, d.name, d.label, d.event_name, o.name, d.object_id::text,
			d.version, d.conditions, d.actions, d.every_seconds, d.next_run_at
		from automation_definitions d
		left join object_definitions o on o.id = d.object_id
		where d.organization_id = $1::uuid and d.name = $2
		for update of d
	`, orgID, name)
	view, err := scanAutomation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return automationView{}, apiErr(http.StatusNotFound, "not found")
	}
	return view, err
}

func scanAutomation(row pgx.Row) (automationView, error) {
	var view automationView
	var eventName, objectName, objectID sql.NullString
	var every sql.NullInt32
	var next sql.NullTime
	var conditions, actions []byte
	if err := row.Scan(&view.ID, &view.Name, &view.Label, &eventName, &objectName, &objectID, &view.Version, &conditions, &actions, &every, &next); err != nil {
		return view, err
	}
	view.Event = eventName.String
	view.Object = objectName.String
	view.ObjectID = objectID.String
	if every.Valid {
		view.Every = formatEvery(int(every.Int32))
	}
	if next.Valid {
		when := next.Time
		view.NextRun = &when
	}
	view.Conditions = []condition{}
	view.Actions = []automationAction{}
	if err := json.Unmarshal(conditions, &view.Conditions); err != nil {
		return view, err
	}
	if err := json.Unmarshal(actions, &view.Actions); err != nil {
		return view, err
	}
	if view.Conditions == nil {
		view.Conditions = []condition{}
	}
	if view.Actions == nil {
		view.Actions = []automationAction{}
	}
	return view, nil
}

// enqueueDueSchedules stores a tick for each workflow whose clock has arrived.
// The request that defined the workflow does not run it.
func (s *server) enqueueDueSchedules(ctx context.Context) (int, error) {
	total := 0
	for {
		n, err := s.enqueueScheduleBatch(ctx, 100)
		total += n
		if err != nil || n == 0 {
			return total, err
		}
	}
}

func (s *server) enqueueScheduleBatch(ctx context.Context, limit int) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		select id::text, organization_id::text, name, actor_id::text
		from automation_definitions
		where every_seconds is not null and next_run_at <= now()
		order by next_run_at, id
		limit $1
		for update skip locked
	`, limit)
	if err != nil {
		return 0, err
	}
	type dueSchedule struct {
		id, org, name, actor string
	}
	due := []dueSchedule{}
	for rows.Next() {
		var item dueSchedule
		if err := rows.Scan(&item.id, &item.org, &item.name, &item.actor); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}
	for _, item := range due {
		eventID, err := insertEvent(ctx, tx, item.org, eventWrite{
			Name:    "schedule.tick",
			ActorID: item.actor,
			Source:  sourceAPI,
			Payload: map[string]any{"automation": item.name},
		})
		if err != nil {
			return 0, err
		}
		if err := enqueueJob(ctx, tx, item.org, eventID, "automation", "", item.id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `
			update automation_definitions
			set next_run_at = now() + make_interval(secs => every_seconds)
			where id = $1::uuid
		`, item.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(due), nil
}

func (s *server) runAutomation(ctx context.Context, job dueJob) error {
	if job.AutomationID == "" {
		return nil
	}
	var conditionsRaw, actionsRaw []byte
	err := s.pool.QueryRow(ctx, `
		select conditions, actions
		from automation_definitions
		where id = $1::uuid and organization_id = $2::uuid
	`, job.AutomationID, job.OrgID).Scan(&conditionsRaw, &actionsRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var existing int
	err = s.pool.QueryRow(ctx, `
		select 1 from automation_executions
		where automation_id = $1::uuid and event_id = $2::uuid
	`, job.AutomationID, job.EventID).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var conditions []condition
	var actions []automationAction
	if err := json.Unmarshal(conditionsRaw, &conditions); err != nil {
		return err
	}
	if err := json.Unmarshal(actionsRaw, &actions); err != nil {
		return err
	}
	event, err := s.loadWebhookEvent(ctx, job.EventID)
	if err != nil {
		return err
	}
	data, err := s.automationRecord(ctx, event)
	if err != nil {
		return err
	}
	matched, err := matchConditions(conditions, data)
	if err != nil {
		return err
	}
	if !matched {
		return s.recordExecution(ctx, job.OrgID, job.AutomationID, job.EventID, "skipped", nil)
	}
	actor, err := s.memberActor(ctx, job.OrgID, event.ActorID)
	if err != nil {
		return err
	}
	created := []string{}
	recordID := ""
	if event.RecordID != nil {
		recordID = *event.RecordID
	}
	sourceName, _ := event.Payload["object"].(string)
	sourceObjectID := ""
	if event.ObjectID != nil {
		sourceObjectID = *event.ObjectID
	}
	for _, action := range actions {
		id, err := s.runAction(ctx, actor, event.Name, sourceName, sourceObjectID, recordID, data, action, job.AutomationID)
		if err != nil {
			return err
		}
		if id != "" {
			created = append(created, id)
		}
	}
	return s.recordExecution(ctx, job.OrgID, job.AutomationID, job.EventID, "done", created)
}

func (s *server) runAction(ctx context.Context, a actor, eventName, sourceName, sourceObjectID, recordID string, record map[string]any, action automationAction, cause string) (string, error) {
	switch action.Type {
	case "create":
		return s.runCreateAction(ctx, a, recordID, record, action, cause)
	case "update":
		return "", s.runUpdateAction(ctx, a, sourceName, sourceObjectID, recordID, record, action, cause)
	case "delete":
		return "", s.runDeleteAction(ctx, a, sourceName, sourceObjectID, recordID, record, action, cause)
	case "http":
		return "", s.runHTTPAction(ctx, eventName, sourceName, recordID, record, action)
	default:
		return "", fmt.Errorf("invalid action")
	}
}

func validateHTTPAction(sourceName string, source map[string]fieldDef, action automationAction) error {
	if err := validHTTPURL(action.URL); err != nil {
		return err
	}
	return walkHTTPValue(sourceName, source, action.Data)
}

func walkHTTPValue(sourceName string, source map[string]fieldDef, value any) error {
	if field, ok := fieldRef(value); ok {
		if _, known := source[field]; !known {
			return apiErrf(http.StatusBadRequest, "unknown field %s", field)
		}
		return nil
	}
	if isRecordRef(value) {
		if sourceName == "" {
			return apiErr(http.StatusBadRequest, "invalid record")
		}
		return nil
	}
	switch typed := value.(type) {
	case nil, string, float64, bool:
		return nil
	case map[string]any:
		for _, child := range typed {
			if err := walkHTTPValue(sourceName, source, child); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for _, child := range typed {
			if err := walkHTTPValue(sourceName, source, child); err != nil {
				return err
			}
		}
		return nil
	default:
		return apiErr(http.StatusBadRequest, "invalid action")
	}
}

func (s *server) runHTTPAction(ctx context.Context, eventName, objectName, recordID string, record map[string]any, action automationAction) error {
	var payload any
	if action.Data != nil {
		resolved, err := resolveHTTPValue(action.Data, recordID, record)
		if err != nil {
			return err
		}
		payload = resolved
	} else {
		payload = map[string]any{
			"event":     eventName,
			"object":    objectName,
			"record_id": recordID,
			"data":      record,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, action.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Openforce-Action")
	resp, err := webhookClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return nil
}

func resolveHTTPValue(value any, recordID string, record map[string]any) (any, error) {
	if field, ok := fieldRef(value); ok {
		return record[field], nil
	}
	if isRecordRef(value) {
		if recordID == "" {
			return nil, fmt.Errorf("invalid record")
		}
		return recordID, nil
	}
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range typed {
			resolved, err := resolveHTTPValue(child, recordID, record)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			resolved, err := resolveHTTPValue(child, recordID, record)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return value, nil
	}
}

func (s *server) actionObject(ctx context.Context, orgID, sourceName, sourceObjectID string, action automationAction) (string, string, error) {
	if action.Object == "" {
		if sourceObjectID == "" || sourceName == "" {
			return "", "", fmt.Errorf("invalid record")
		}
		return sourceObjectID, sourceName, nil
	}
	id, err := s.objectByName(ctx, orgID, action.Object)
	if err != nil {
		return "", "", err
	}
	return id, action.Object, nil
}

func (s *server) runUpdateAction(ctx context.Context, a actor, sourceName, sourceObjectID, recordID string, record map[string]any, action automationAction, cause string) error {
	objectID, objectName, err := s.actionObject(ctx, a.OrgID, sourceName, sourceObjectID, action)
	if err != nil {
		return err
	}
	if err := s.authorize(ctx, a, objectID, true); err != nil {
		return err
	}
	id, err := resolveTargetID(action, recordID, record)
	if err != nil {
		return err
	}
	patch, err := resolveActionData(action.Data, recordID, record)
	if err != nil {
		return err
	}
	_, err = s.writeRecordPatch(ctx, a, objectID, objectName, id, patch, sourceAutomation, cause)
	return err
}

func (s *server) runDeleteAction(ctx context.Context, a actor, sourceName, sourceObjectID, recordID string, record map[string]any, action automationAction, cause string) error {
	objectID, objectName, err := s.actionObject(ctx, a.OrgID, sourceName, sourceObjectID, action)
	if err != nil {
		return err
	}
	if err := s.authorize(ctx, a, objectID, true); err != nil {
		return err
	}
	id, err := resolveTargetID(action, recordID, record)
	if err != nil {
		return err
	}
	return s.writeRecordDelete(ctx, a, objectID, objectName, id, sourceAutomation, cause)
}

func resolveTargetID(action automationAction, triggerID string, record map[string]any) (string, error) {
	if action.Record == nil || isRecordRef(action.Record) {
		if _, err := uuid.Parse(triggerID); err != nil {
			return "", fmt.Errorf("invalid record")
		}
		return triggerID, nil
	}
	field, ok := fieldRef(action.Record)
	if !ok {
		return "", fmt.Errorf("invalid record")
	}
	id, _ := record[field].(string)
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("invalid record")
	}
	return id, nil
}

func (s *server) automationRecord(ctx context.Context, event webhookEvent) (map[string]any, error) {
	if data, ok := payloadData(event.Payload); ok {
		return data, nil
	}
	if event.RecordID == nil {
		return map[string]any{}, nil
	}
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select data from records
		where id = $1::uuid and organization_id = $2::uuid
	`, *event.RecordID, event.OrganizationID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func payloadData(payload map[string]any) (map[string]any, bool) {
	raw, ok := payload["data"]
	if !ok || raw == nil {
		return nil, false
	}
	data, ok := raw.(map[string]any)
	return data, ok
}

func (s *server) memberActor(ctx context.Context, orgID, userID string) (actor, error) {
	var role string
	err := s.pool.QueryRow(ctx, `
		select role from memberships
		where organization_id = $1::uuid and user_id = $2::uuid
	`, orgID, userID).Scan(&role)
	if err != nil {
		return actor{}, err
	}
	return actor{UserID: userID, OrgID: orgID, Role: role}, nil
}

func (s *server) runCreateAction(ctx context.Context, a actor, recordID string, record map[string]any, action automationAction, cause string) (string, error) {
	objectID, err := s.objectByName(ctx, a.OrgID, action.Object)
	if err != nil {
		return "", err
	}
	if err := s.authorize(ctx, a, objectID, true); err != nil {
		return "", err
	}
	data, err := resolveActionData(action.Data, recordID, record)
	if err != nil {
		return "", err
	}
	defs, err := s.fieldDefs(ctx, objectID)
	if err != nil {
		return "", err
	}
	if err := validateData(defs, data, true); err != nil {
		return "", err
	}
	if err := s.checkRelations(ctx, a.OrgID, objectID, defs, data); err != nil {
		return "", err
	}
	view, err := s.insertRecord(ctx, a, objectID, action.Object, defs, data, sourceAutomation, cause)
	if err != nil {
		return "", err
	}
	return view.ID, nil
}

func resolveActionData(data map[string]any, recordID string, record map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for key, value := range data {
		if field, ok := fieldRef(value); ok {
			out[key] = record[field]
			continue
		}
		if isRecordRef(value) {
			if recordID == "" {
				return nil, fmt.Errorf("invalid record")
			}
			out[key] = recordID
			continue
		}
		out[key] = value
	}
	return out, nil
}

func matchConditions(conditions []condition, data map[string]any) (bool, error) {
	for _, cond := range conditions {
		value, ok := data[cond.Field]
		if !ok || value == nil {
			return false, nil
		}
		matched, err := compareCondition(cond.Op, value, cond.Value)
		if err != nil || !matched {
			return false, err
		}
	}
	return true, nil
}

func compareCondition(op string, left, right any) (bool, error) {
	switch op {
	case "eq":
		return reflect.DeepEqual(left, right), nil
	case "neq":
		return !reflect.DeepEqual(left, right), nil
	case "contains":
		text, ok := left.(string)
		needle, ok2 := right.(string)
		if !ok || !ok2 {
			return false, fmt.Errorf("invalid value")
		}
		return strings.Contains(strings.ToLower(text), strings.ToLower(needle)), nil
	case "gt", "gte", "lt", "lte":
		order, err := compareOrdered(left, right)
		if err != nil {
			return false, err
		}
		switch op {
		case "gt":
			return order > 0, nil
		case "gte":
			return order >= 0, nil
		case "lt":
			return order < 0, nil
		default:
			return order <= 0, nil
		}
	default:
		return false, fmt.Errorf("invalid operator")
	}
}

func compareOrdered(left, right any) (int, error) {
	if a, ok := asFloat(left); ok {
		b, ok := asFloat(right)
		if !ok {
			return 0, fmt.Errorf("invalid value")
		}
		switch {
		case a < b:
			return -1, nil
		case a > b:
			return 1, nil
		default:
			return 0, nil
		}
	}
	a, aok := left.(string)
	b, bok := right.(string)
	if !aok || !bok {
		return 0, fmt.Errorf("invalid value")
	}
	switch {
	case a < b:
		return -1, nil
	case a > b:
		return 1, nil
	default:
		return 0, nil
	}
}

func asFloat(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		parsed, err := n.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (s *server) recordExecution(ctx context.Context, orgID, automationID, eventID, status string, created []string) error {
	if created == nil {
		created = []string{}
	}
	raw, err := json.Marshal(created)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		insert into automation_executions
			(id, organization_id, automation_id, event_id, status, created_ids)
		values ($1, $2, $3, $4, $5, $6::jsonb)
	`, uuid.NewString(), orgID, automationID, eventID, status, string(raw))
	if isUnique(err) {
		return nil
	}
	return err
}
