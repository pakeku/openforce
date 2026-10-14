package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type emailView struct {
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type emailWrite struct {
	To      string
	Subject string
	Body    string
}

// listEmails is owner-only. Rows are stored messages; nothing here is sent.
func (s *server) listEmails(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := uuid.Parse(a.OrgID); err != nil {
		writeErr(w, apiErr(http.StatusBadRequest, "not found"))
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		select to_address, subject, body, created_at
		from outbound_emails
		where organization_id = $1::uuid
		order by created_at desc, id desc
	`, a.OrgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	views := []emailView{}
	for rows.Next() {
		var view emailView
		if err := rows.Scan(&view.To, &view.Subject, &view.Body, &view.CreatedAt); err != nil {
			writeErr(w, err)
			return
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"emails": views})
}

func validateEmailAction(sourceName string, source map[string]fieldDef, action automationAction) error {
	if action.Name != "" || action.Object != "" || action.URL != "" || action.User != "" || action.Title != nil || action.Record != nil || len(action.Data) > 0 {
		return apiErr(http.StatusBadRequest, "invalid action")
	}
	if err := validateEmailText(sourceName, source, action.To, "to"); err != nil {
		return err
	}
	if err := validateEmailText(sourceName, source, action.Subject, "subject"); err != nil {
		return err
	}
	return validateEmailText(sourceName, source, action.Body, "")
}

func validateEmailText(sourceName string, source map[string]fieldDef, value any, required string) error {
	if value == nil {
		if required != "" {
			return apiErrf(http.StatusBadRequest, "%s is required", required)
		}
		return nil
	}
	if text, ok := value.(string); ok {
		if required != "" && text == "" {
			return apiErrf(http.StatusBadRequest, "%s is required", required)
		}
		return nil
	}
	if field, ok := fieldRef(value); ok {
		if sourceName == "" {
			return apiErr(http.StatusBadRequest, "invalid record")
		}
		if _, known := source[field]; !known {
			return apiErrf(http.StatusBadRequest, "unknown field %s", field)
		}
		return nil
	}
	if isRecordRef(value) {
		return apiErr(http.StatusBadRequest, "invalid record")
	}
	return apiErr(http.StatusBadRequest, "invalid action")
}

func prepareEmail(recordID string, record map[string]any, action automationAction) (emailWrite, error) {
	to, err := resolveEmailText(action.To, recordID, record)
	if err != nil {
		return emailWrite{}, err
	}
	if to == "" {
		return emailWrite{}, fmt.Errorf("to is required")
	}
	subject, err := resolveEmailText(action.Subject, recordID, record)
	if err != nil {
		return emailWrite{}, err
	}
	if subject == "" {
		return emailWrite{}, fmt.Errorf("subject is required")
	}
	body, err := resolveEmailText(action.Body, recordID, record)
	if err != nil {
		return emailWrite{}, err
	}
	return emailWrite{To: to, Subject: subject, Body: body}, nil
}

func resolveEmailText(value any, recordID string, record map[string]any) (string, error) {
	if value == nil {
		return "", nil
	}
	if text, ok := value.(string); ok {
		return text, nil
	}
	if _, ok := fieldRef(value); ok {
		resolved, err := resolveHTTPValue(value, recordID, record)
		if err != nil {
			return "", err
		}
		return notificationScalar(resolved)
	}
	if isRecordRef(value) {
		return "", fmt.Errorf("invalid record")
	}
	return "", fmt.Errorf("invalid action")
}

// commitEmails stores the messages with the execution. Nothing is sent.
// A conflicting execution rolls the inserts back, so a second pass does not duplicate them.
func (s *server) commitEmails(ctx context.Context, orgID, automationID, eventID, path string, created []string, notes []notificationWrite, emails []emailWrite) error {
	if _, err := uuid.Parse(orgID); err != nil {
		return fmt.Errorf("not found")
	}
	if _, err := uuid.Parse(automationID); err != nil {
		return fmt.Errorf("not found")
	}
	if _, err := uuid.Parse(eventID); err != nil {
		return fmt.Errorf("invalid record")
	}
	if created == nil {
		created = []string{}
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		return err
	}
	raw := string(encoded)
	var storedPath any
	if path != "" {
		storedPath = path
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	base := time.Now().UTC()
	stamp := 0
	for _, note := range notes {
		if _, err := uuid.Parse(note.UserID); err != nil {
			return fmt.Errorf("not found")
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
	_, err = tx.Exec(ctx, `
		insert into automation_executions
			(id, organization_id, automation_id, event_id, status, path, created_ids)
		values ($1, $2::uuid, $3::uuid, $4::uuid, 'done', $5, $6::jsonb)
	`, uuid.NewString(), orgID, automationID, eventID, storedPath, raw)
	if isUnique(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
