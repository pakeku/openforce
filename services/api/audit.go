package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type auditView struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	ActorID   string    `json:"actor_id"`
	Name      string    `json:"name"`
	Field     *string   `json:"field"`
	Before    any       `json:"before"`
	After     any       `json:"after"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type pendingAudit struct {
	ID        string
	OrgID     string
	ActorID   string
	ObjectID  string
	RecordID  string
	Name      string
	Source    string
	Payload   map[string]any
	CreatedAt time.Time
}

// deliverAudit copies stored events into the audit log. Request handlers do not call it.
func (s *server) deliverAudit(ctx context.Context) (int, error) {
	delivered := 0
	for {
		n, scanned, err := s.deliverAuditBatch(ctx, 100)
		delivered += n
		if err != nil || scanned == 0 {
			return delivered, err
		}
	}
}

func (s *server) deliverAuditBatch(ctx context.Context, limit int) (int, int, error) {
	rows, err := s.pool.Query(ctx, `
		select e.id::text, e.organization_id::text, e.actor_id::text,
			e.object_id::text, e.record_id::text,
			e.name, e.source, e.payload, e.created_at
		from events e
		where e.name in ('field.changed', 'record.deleted')
			and e.object_id is not null
			and e.record_id is not null
			and not exists (select 1 from audit_entries a where a.event_id = e.id)
		order by e.seq
		limit $1
	`, limit)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	pending := []pendingAudit{}
	for rows.Next() {
		var ev pendingAudit
		var objectID, recordID sql.NullString
		var raw []byte
		if err := rows.Scan(&ev.ID, &ev.OrgID, &ev.ActorID, &objectID, &recordID, &ev.Name, &ev.Source, &raw, &ev.CreatedAt); err != nil {
			return 0, 0, err
		}
		if !objectID.Valid || !recordID.Valid {
			return 0, 0, fmt.Errorf("audit event %s is missing its record", ev.ID)
		}
		ev.ObjectID = objectID.String
		ev.RecordID = recordID.String
		ev.Payload = map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &ev.Payload); err != nil {
				return 0, 0, err
			}
		}
		pending = append(pending, ev)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	delivered := 0
	for _, ev := range pending {
		ok, err := s.insertAudit(ctx, ev)
		if err != nil {
			return delivered, len(pending), err
		}
		if ok {
			delivered++
		}
	}
	return delivered, len(pending), nil
}

func (s *server) insertAudit(ctx context.Context, ev pendingAudit) (bool, error) {
	var field any
	includeValues := false
	if ev.Name == "field.changed" {
		name, _ := ev.Payload["field"].(string)
		if name == "" {
			return false, fmt.Errorf("field.changed %s is missing a field", ev.ID)
		}
		field = name
		includeValues = true
	}
	before, err := jsonbValue(includeValues, ev.Payload["before"])
	if err != nil {
		return false, err
	}
	after, err := jsonbValue(includeValues, ev.Payload["after"])
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		insert into audit_entries (
			id, organization_id, event_id, actor_id, object_id, record_id,
			name, field, before, after, source, created_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11, $12)
	`, uuid.NewString(), ev.OrgID, ev.ID, ev.ActorID, ev.ObjectID, ev.RecordID, ev.Name, field, before, after, ev.Source, ev.CreatedAt)
	if isUnique(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func jsonbValue(include bool, value any) (any, error) {
	if !include {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func (s *server) listAudit(w http.ResponseWriter, r *http.Request) {
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
	recordID := r.PathValue("id")
	if _, err := uuid.Parse(recordID); err != nil {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	var one int
	err = s.pool.QueryRow(r.Context(), `
		select 1 from records
		where id = $1::uuid and organization_id = $2::uuid and object_id = $3::uuid
	`, recordID, a.OrgID, objectID).Scan(&one)
	if err == pgx.ErrNoRows {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	entries, err := s.selectAudit(r.Context(), a.OrgID, objectID, recordID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *server) selectAudit(ctx context.Context, orgID, objectID, recordID string) ([]auditView, error) {
	rows, err := s.pool.Query(ctx, `
		select a.id::text, a.event_id::text, a.actor_id::text, a.name, a.field,
			a.before::text, a.after::text, a.source, a.created_at
		from audit_entries a
		join events e on e.id = a.event_id
		where a.organization_id = $1::uuid and a.object_id = $2::uuid and a.record_id = $3::uuid
		order by e.seq
	`, orgID, objectID, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []auditView{}
	for rows.Next() {
		var view auditView
		var field, before, after sql.NullString
		if err := rows.Scan(&view.ID, &view.EventID, &view.ActorID, &view.Name, &field, &before, &after, &view.Source, &view.CreatedAt); err != nil {
			return nil, err
		}
		view.Field = nullText(field)
		decoded, err := decodeJSONValue(before)
		if err != nil {
			return nil, err
		}
		view.Before = decoded
		decoded, err = decodeJSONValue(after)
		if err != nil {
			return nil, err
		}
		view.After = decoded
		entries = append(entries, view)
	}
	return entries, rows.Err()
}

func decodeJSONValue(v sql.NullString) (any, error) {
	if !v.Valid {
		return nil, nil
	}
	var out any
	if err := json.Unmarshal([]byte(v.String), &out); err != nil {
		return nil, err
	}
	return out, nil
}
