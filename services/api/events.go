package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	sourceAPI        = "API"
	sourceAdmin      = "Admin"
	sourceAutomation = "Automation"
)

var eventNames = map[string]struct{}{
	"record.created":     {},
	"record.updated":     {},
	"record.deleted":     {},
	"field.changed":      {},
	"user.created":       {},
	"object.created":     {},
	"workflow.started":   {},
	"workflow.completed": {},
}

type eventWrite struct {
	Name     string
	ActorID  string
	ObjectID string
	RecordID string
	Source   string
	Payload  map[string]any
}

type eventView struct {
	ID        string         `json:"id"`
	Seq       int64          `json:"seq"`
	Name      string         `json:"name"`
	ActorID   string         `json:"actor_id"`
	ObjectID  *string        `json:"object_id"`
	RecordID  *string        `json:"record_id"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type fieldChange struct {
	Name   string
	Before any
	After  any
}

func knownEvent(name string) bool {
	_, ok := eventNames[name]
	return ok
}

// insertEvent participates in the caller's transaction, so the event is stored only when that change commits.
func insertEvent(ctx context.Context, tx pgx.Tx, orgID string, ev eventWrite) error {
	if ev.Payload == nil {
		ev.Payload = map[string]any{}
	}
	raw, err := json.Marshal(ev.Payload)
	if err != nil {
		return err
	}
	eventID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		insert into events (id, organization_id, name, actor_id, object_id, record_id, source, payload)
		values ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
	`, eventID, orgID, ev.Name, ev.ActorID, uuidOrNil(ev.ObjectID), uuidOrNil(ev.RecordID), ev.Source, string(raw))
	if err != nil {
		return err
	}
	return enqueueDeliveries(ctx, tx, orgID, eventID, ev.Name, ev.ObjectID)
}

func enqueueDeliveries(ctx context.Context, tx pgx.Tx, orgID, eventID, name, objectID string) error {
	if name == "field.changed" || name == "record.deleted" {
		if err := enqueueJob(ctx, tx, orgID, eventID, "audit", "", ""); err != nil {
			return err
		}
	}
	if err := enqueueWebhookJobs(ctx, tx, orgID, eventID, name); err != nil {
		return err
	}
	return enqueueAutomationJobs(ctx, tx, orgID, eventID, name, objectID)
}

func uuidOrNil(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func fieldChanges(defs []fieldDef, before, patch map[string]any) []fieldChange {
	changes := []fieldChange{}
	for _, def := range defs {
		after, ok := patch[def.Name]
		if !ok {
			continue
		}
		prev, exists := before[def.Name]
		if !exists {
			prev = nil
		}
		if reflect.DeepEqual(prev, after) {
			continue
		}
		changes = append(changes, fieldChange{Name: def.Name, Before: prev, After: after})
	}
	return changes
}

func emitFieldChanges(ctx context.Context, tx pgx.Tx, orgID, actorID, objectID, recordID, objectName, source string, changes []fieldChange) error {
	for _, change := range changes {
		err := insertEvent(ctx, tx, orgID, eventWrite{
			Name:     "field.changed",
			ActorID:  actorID,
			ObjectID: objectID,
			RecordID: recordID,
			Source:   source,
			Payload: map[string]any{
				"object": objectName,
				"field":  change.Name,
				"before": change.Before,
				"after":  change.After,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	name := r.URL.Query().Get("name")
	if name != "" && !knownEvent(name) {
		writeErr(w, apiErr(http.StatusBadRequest, "unknown event"))
		return
	}
	recordID := r.URL.Query().Get("record")
	if recordID != "" {
		if _, err := uuid.Parse(recordID); err != nil {
			writeErr(w, apiErr(http.StatusBadRequest, "invalid record"))
			return
		}
	}
	after, err := eventAfter(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	limit, err := listLimit(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	events, err := s.selectEvents(r.Context(), a, name, recordID, after, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func eventAfter(r *http.Request) (int64, error) {
	v := r.URL.Query().Get("after")
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, apiErr(http.StatusBadRequest, "invalid after")
	}
	return n, nil
}

func (s *server) selectEvents(ctx context.Context, a actor, name, recordID string, after int64, limit int) ([]eventView, error) {
	args := []any{a.OrgID, after}
	query := `
		select e.id::text, e.seq, e.name, e.actor_id::text,
			e.object_id::text, e.record_id::text,
			e.source, e.payload, e.created_at
		from events e
		where e.organization_id = $1::uuid
			and e.seq > $2::bigint`
	if name != "" {
		args = append(args, name)
		query += fmt.Sprintf(" and e.name = $%d", len(args))
	}
	if recordID != "" {
		args = append(args, recordID)
		query += fmt.Sprintf(" and e.record_id = $%d::uuid", len(args))
	}
	args = append(args, a.Role)
	query += fmt.Sprintf(`
			and (
				$%d = 'owner'
				or e.name not in ('record.created', 'record.updated', 'record.deleted', 'field.changed')
				or exists (
					select 1 from object_permissions p
					where p.organization_id = e.organization_id
						and p.object_id = e.object_id
						and p.role = 'member'
						and p.can_read
				)
			)
		order by e.seq`, len(args))
	args = append(args, limit)
	query += fmt.Sprintf(" limit $%d", len(args))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []eventView{}
	for rows.Next() {
		var view eventView
		var objectID, recordID sql.NullString
		var raw []byte
		if err := rows.Scan(&view.ID, &view.Seq, &view.Name, &view.ActorID, &objectID, &recordID, &view.Source, &raw, &view.CreatedAt); err != nil {
			return nil, err
		}
		view.ObjectID = nullText(objectID)
		view.RecordID = nullText(recordID)
		view.Payload = map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &view.Payload); err != nil {
				return nil, err
			}
		}
		events = append(events, view)
	}
	return events, rows.Err()
}

func nullText(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
