package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type notificationView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type notificationWrite struct {
	UserID string
	Title  string
	Body   string
}

func (s *server) listNotifications(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		select id::text, title, body, created_at
		from notifications
		where organization_id = $1::uuid and user_id = $2::uuid
		order by created_at desc, id desc
	`, a.OrgID, a.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	views := []notificationView{}
	for rows.Next() {
		var view notificationView
		if err := rows.Scan(&view.ID, &view.Title, &view.Body, &view.CreatedAt); err != nil {
			writeErr(w, err)
			return
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": views})
}

func (s *server) validateNotificationAction(ctx context.Context, orgID, sourceName string, source map[string]fieldDef, action automationAction) error {
	if action.Name != "" || action.Object != "" || action.URL != "" || action.Record != nil || len(action.Data) > 0 {
		return apiErr(http.StatusBadRequest, "invalid action")
	}
	if strings.TrimSpace(action.User) == "" {
		return apiErr(http.StatusBadRequest, "user is required")
	}
	if _, err := s.notificationUser(ctx, orgID, action.User); err != nil {
		return err
	}
	if err := validateNotificationText(sourceName, source, action.Title, true); err != nil {
		return err
	}
	return validateNotificationText(sourceName, source, action.Body, false)
}

func validateNotificationText(sourceName string, source map[string]fieldDef, value any, required bool) error {
	if value == nil {
		if required {
			return apiErr(http.StatusBadRequest, "title is required")
		}
		return nil
	}
	if text, ok := value.(string); ok {
		if required && text == "" {
			return apiErr(http.StatusBadRequest, "title is required")
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
		if sourceName == "" {
			return apiErr(http.StatusBadRequest, "invalid record")
		}
		return nil
	}
	return apiErr(http.StatusBadRequest, "invalid action")
}

func (s *server) notificationUser(ctx context.Context, orgID, email string) (string, error) {
	if _, err := uuid.Parse(orgID); err != nil {
		return "", apiErr(http.StatusBadRequest, "not found")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", apiErr(http.StatusBadRequest, "user is required")
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		select u.id::text
		from users u
		join memberships m on m.user_id = u.id and m.organization_id = $2::uuid
		where u.email = $1
	`, email, orgID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apiErr(http.StatusBadRequest, "not found")
	}
	if err != nil {
		return "", err
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", apiErr(http.StatusBadRequest, "not found")
	}
	return id, nil
}

func (s *server) prepareNotification(ctx context.Context, orgID, recordID string, record map[string]any, action automationAction) (notificationWrite, error) {
	userID, err := s.notificationUser(ctx, orgID, action.User)
	if err != nil {
		return notificationWrite{}, err
	}
	title, err := resolveNotificationText(action.Title, recordID, record)
	if err != nil {
		return notificationWrite{}, err
	}
	if title == "" {
		return notificationWrite{}, fmt.Errorf("title is required")
	}
	body, err := resolveNotificationText(action.Body, recordID, record)
	if err != nil {
		return notificationWrite{}, err
	}
	return notificationWrite{UserID: userID, Title: title, Body: body}, nil
}

func resolveNotificationText(value any, recordID string, record map[string]any) (string, error) {
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
		if _, err := uuid.Parse(recordID); err != nil {
			return "", fmt.Errorf("invalid record")
		}
		return recordID, nil
	}
	return "", fmt.Errorf("invalid action")
}

func notificationScalar(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("invalid action")
	}
}

// commitNotifications stores the rows with the execution. A failed action returns before this.
// now() is frozen for the transaction, so each row gets its own timestamp.
func (s *server) commitNotifications(ctx context.Context, orgID, automationID, eventID, path string, created []string, notes []notificationWrite) error {
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
	for i, note := range notes {
		if _, err := uuid.Parse(note.UserID); err != nil {
			return fmt.Errorf("not found")
		}
		if _, err := tx.Exec(ctx, `
			insert into notifications
				(id, organization_id, user_id, title, body, event_id, created_at)
			values ($1, $2::uuid, $3::uuid, $4, $5, $6::uuid, $7)
		`, uuid.NewString(), orgID, note.UserID, note.Title, note.Body, eventID, base.Add(time.Duration(i)*time.Millisecond)); err != nil {
			return err
		}
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
