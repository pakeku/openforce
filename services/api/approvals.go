package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type approvalView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// actionError is a step that failed while a person was deciding.
// The execution stays waiting and the response keeps this message.
type actionError struct {
	err error
}

func (e *actionError) Error() string { return e.err.Error() }

func (e *actionError) Unwrap() error { return e.err }

func (s *server) listApprovals(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if _, err := uuid.Parse(a.OrgID); err != nil {
		writeErr(w, apiErr(http.StatusBadRequest, "not found"))
		return
	}
	if _, err := uuid.Parse(a.UserID); err != nil {
		writeErr(w, apiErr(http.StatusBadRequest, "not found"))
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		select a.id::text, d.name, a.created_at
		from approvals a
		join automation_definitions d on d.id = a.automation_id
		join automation_executions e
		  on e.automation_id = a.automation_id and e.event_id = a.event_id
		where a.organization_id = $1::uuid
		  and a.user_id = $2::uuid
		  and e.status = 'waiting'
		order by a.created_at desc, a.id desc
	`, a.OrgID, a.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	views := []approvalView{}
	for rows.Next() {
		var view approvalView
		if err := rows.Scan(&view.ID, &view.Name, &view.CreatedAt); err != nil {
			writeErr(w, err)
			return
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": views})
}

func (s *server) decideApproval(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	var body struct {
		Decision string `json:"decision"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Decision != "approve" && body.Decision != "reject" {
		writeErr(w, apiErr(http.StatusBadRequest, "invalid decision"))
		return
	}
	var automationID, eventID, userID string
	err := s.pool.QueryRow(r.Context(), `
		select automation_id::text, event_id::text, user_id::text
		from approvals
		where id = $1::uuid and organization_id = $2::uuid
	`, id, a.OrgID).Scan(&automationID, &eventID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := uuid.Parse(automationID); err != nil {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if _, err := uuid.Parse(eventID); err != nil {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if userID != a.UserID {
		writeErr(w, apiErr(http.StatusForbidden, "forbidden"))
		return
	}
	view, err := s.continueApproval(r.Context(), a.OrgID, automationID, eventID, body.Decision)
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func writeActionErr(w http.ResponseWriter, err error) {
	var action *actionError
	if errors.As(err, &action) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": action.Error()})
		return
	}
	writeErr(w, err)
}

func (s *server) validateApprovalAction(ctx context.Context, orgID string, action automationAction) error {
	if action.Name != "" || action.Object != "" || action.URL != "" || action.Record != nil || len(action.Data) > 0 || action.Title != nil || action.Body != nil {
		return apiErr(http.StatusBadRequest, "invalid action")
	}
	if strings.TrimSpace(action.User) == "" {
		return apiErr(http.StatusBadRequest, "user is required")
	}
	_, err := s.notificationUser(ctx, orgID, action.User)
	return err
}

// continueApproval runs the rest of the workflow in this request.
// It does not enqueue another automation job. A failed step leaves the execution waiting.
func (s *server) continueApproval(ctx context.Context, orgID, automationID, eventID, decision string) (executionView, error) {
	if _, err := uuid.Parse(orgID); err != nil {
		return executionView{}, apiErr(http.StatusNotFound, "not found")
	}
	if _, err := uuid.Parse(automationID); err != nil {
		return executionView{}, apiErr(http.StatusNotFound, "not found")
	}
	if _, err := uuid.Parse(eventID); err != nil {
		return executionView{}, apiErr(http.StatusNotFound, "not found")
	}
	var actionsRaw, elseRaw []byte
	err := s.pool.QueryRow(ctx, `
		select actions, else_actions
		from automation_definitions
		where id = $1::uuid and organization_id = $2::uuid
	`, automationID, orgID).Scan(&actionsRaw, &elseRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return executionView{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return executionView{}, err
	}
	var actions, elseActions []automationAction
	if err := json.Unmarshal(actionsRaw, &actions); err != nil {
		return executionView{}, err
	}
	if err := json.Unmarshal(elseRaw, &elseActions); err != nil {
		return executionView{}, err
	}
	event, err := s.loadWebhookEvent(ctx, eventID)
	if err != nil {
		return executionView{}, err
	}
	if event.OrganizationID != orgID {
		return executionView{}, apiErr(http.StatusNotFound, "not found")
	}
	data, err := s.automationRecord(ctx, event)
	if err != nil {
		return executionView{}, err
	}
	actor, err := s.memberActor(ctx, orgID, event.ActorID)
	if err != nil {
		return executionView{}, err
	}
	recordID := ""
	if event.RecordID != nil {
		recordID = *event.RecordID
	}
	sourceName, _ := event.Payload["object"].(string)
	sourceObjectID := ""
	if event.ObjectID != nil {
		sourceObjectID = *event.ObjectID
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return executionView{}, err
	}
	defer tx.Rollback(ctx)
	var execID, status string
	var createdRaw []byte
	err = tx.QueryRow(ctx, `
		select id::text, status, created_ids
		from automation_executions
		where organization_id = $1::uuid and automation_id = $2::uuid and event_id = $3::uuid
		for update
	`, orgID, automationID, eventID).Scan(&execID, &status, &createdRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return executionView{}, apiErr(http.StatusNotFound, "not found")
	}
	if err != nil {
		return executionView{}, err
	}
	if _, err := uuid.Parse(execID); err != nil {
		return executionView{}, apiErr(http.StatusNotFound, "not found")
	}
	if status != "waiting" {
		return executionView{}, apiErr(http.StatusConflict, "already decided")
	}
	prior, err := decodeIDs(createdRaw)
	if err != nil {
		return executionView{}, err
	}

	chosen := []automationAction{}
	nextStatus := "done"
	nextPath := ""
	switch decision {
	case "approve":
		_, after, step, err := splitApproval(actions)
		if err != nil {
			return executionView{}, apiErr(http.StatusBadRequest, "invalid action")
		}
		if step != nil {
			chosen = after
		}
		nextPath = "actions"
	case "reject":
		if len(elseActions) == 0 {
			nextStatus = "rejected"
		} else {
			chosen = elseActions
			nextPath = "else"
		}
	default:
		return executionView{}, apiErr(http.StatusBadRequest, "invalid decision")
	}
	result, err := s.runActionList(ctx, actor, event.Name, sourceName, sourceObjectID, recordID, data, chosen, automationID)
	if err != nil {
		var api *apiError
		if errors.As(err, &api) {
			return executionView{}, err
		}
		return executionView{}, &actionError{err: err}
	}
	created := append(prior, result.created...)
	if created == nil {
		created = []string{}
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		return executionView{}, err
	}
	if err := insertActionNotes(ctx, tx, orgID, eventID, result.notes, result.emails); err != nil {
		return executionView{}, err
	}
	var storedPath any
	if nextPath != "" {
		storedPath = nextPath
	}
	var view executionView
	var stored sql.NullString
	err = tx.QueryRow(ctx, `
		update automation_executions
		set status = $1, path = $2, created_ids = $3::jsonb
		where id = $4::uuid and status = 'waiting'
		returning id::text, event_id::text, status, path, created_ids, created_at
	`, nextStatus, storedPath, string(encoded), execID).Scan(
		&view.ID, &view.EventID, &view.Status, &stored, &createdRaw, &view.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return executionView{}, apiErr(http.StatusConflict, "already decided")
	}
	if err != nil {
		return executionView{}, err
	}
	view.Path = stored.String
	view.Created, err = decodeIDs(createdRaw)
	if err != nil {
		return executionView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return executionView{}, err
	}
	return view, nil
}

// commitWaiting stores the approval with the execution. The job then finishes, so it is not due.
func (s *server) commitWaiting(ctx context.Context, orgID, automationID, eventID, userID string, created []string, notes []notificationWrite, emails []emailWrite) error {
	if _, err := uuid.Parse(orgID); err != nil {
		return apiErr(http.StatusBadRequest, "not found")
	}
	if _, err := uuid.Parse(automationID); err != nil {
		return apiErr(http.StatusBadRequest, "not found")
	}
	if _, err := uuid.Parse(eventID); err != nil {
		return fmt.Errorf("invalid record")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return apiErr(http.StatusBadRequest, "not found")
	}
	if created == nil {
		created = []string{}
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		return err
	}
	raw := string(encoded)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := insertActionNotes(ctx, tx, orgID, eventID, notes, emails); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		insert into automation_executions
			(id, organization_id, automation_id, event_id, status, path, created_ids)
		values ($1, $2::uuid, $3::uuid, $4::uuid, 'waiting', null, $5::jsonb)
	`, uuid.NewString(), orgID, automationID, eventID, raw)
	if isUnique(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		insert into approvals
			(id, organization_id, automation_id, event_id, user_id)
		values ($1, $2::uuid, $3::uuid, $4::uuid, $5::uuid)
	`, uuid.NewString(), orgID, automationID, eventID, userID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertActionNotes(ctx context.Context, tx pgx.Tx, orgID, eventID string, notes []notificationWrite, emails []emailWrite) error {
	base := time.Now().UTC()
	stamp := 0
	for _, note := range notes {
		if _, err := uuid.Parse(note.UserID); err != nil {
			return apiErr(http.StatusBadRequest, "not found")
		}
		if _, err := tx.Exec(ctx, `
			insert into notifications
				(id, organization_id, user_id, title, body, event_id, created_at)
			values ($1, $2::uuid, $3::uuid, $4, $5, $6::uuid, $7)
		`, uuid.NewString(), orgID, note.UserID, note.Title, note.Body, eventID, base.Add(time.Duration(stamp)*time.Millisecond)); err != nil {
			return err
		}
		stamp++
	}
	for _, message := range emails {
		if _, err := tx.Exec(ctx, `
			insert into outbound_emails
				(id, organization_id, to_address, subject, body, event_id, created_at)
			values ($1, $2::uuid, $3, $4, $5, $6::uuid, $7)
		`, uuid.NewString(), orgID, message.To, message.Subject, message.Body, eventID, base.Add(time.Duration(stamp)*time.Millisecond)); err != nil {
			return err
		}
		stamp++
	}
	return nil
}

func decodeIDs(raw []byte) ([]string, error) {
	ids := []string{}
	if len(raw) == 0 {
		return ids, nil
	}
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}
