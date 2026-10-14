package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"database/sql"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var webhookClient = &http.Client{
	Timeout: 5 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type webhookInput struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type webhookView struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret,omitempty"`
}

type webhookEvent struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	OrganizationID string         `json:"organization_id"`
	ActorID        string         `json:"actor_id"`
	ObjectID       *string        `json:"object_id"`
	RecordID       *string        `json:"record_id"`
	Source         string         `json:"source"`
	Payload        map[string]any `json:"payload"`
	CreatedAt      time.Time      `json:"created_at"`
}

func enqueueWebhookJobs(ctx context.Context, tx pgx.Tx, orgID, eventID, name string) error {
	rows, err := tx.Query(ctx, `
		select id::text
		from webhook_subscriptions
		where organization_id = $1::uuid
			and events @> jsonb_build_array($2::text)
	`, orgID, name)
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
		if err := enqueueJob(ctx, tx, orgID, eventID, "webhook", id, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) createWebhook(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body webhookInput
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateWebhook(body); err != nil {
		writeErr(w, err)
		return
	}
	secret, err := newSecret()
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, err := json.Marshal(body.Events)
	if err != nil {
		writeErr(w, err)
		return
	}
	view := webhookView{ID: uuid.NewString(), URL: body.URL, Events: body.Events, Secret: secret}
	_, err = s.pool.Exec(r.Context(), `
		insert into webhook_subscriptions (id, organization_id, url, secret, events)
		values ($1, $2, $3, $4, $5::jsonb)
	`, view.ID, a.OrgID, view.URL, secret, string(raw))
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "webhook already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		select id::text, url, events
		from webhook_subscriptions
		where organization_id = $1::uuid
		order by created_at, url
	`, a.OrgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	hooks := []webhookView{}
	for rows.Next() {
		var view webhookView
		var raw []byte
		if err := rows.Scan(&view.ID, &view.URL, &raw); err != nil {
			writeErr(w, err)
			return
		}
		if err := json.Unmarshal(raw, &view.Events); err != nil {
			writeErr(w, err)
			return
		}
		hooks = append(hooks, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": hooks})
}

func (s *server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		delete from webhook_subscriptions
		where id = $1::uuid and organization_id = $2::uuid
	`, r.PathValue("id"), a.OrgID)
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

func validateWebhook(body webhookInput) error {
	parsed, err := url.Parse(body.URL)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return apiErr(http.StatusBadRequest, "invalid url")
	}
	if len(body.Events) == 0 {
		return apiErr(http.StatusBadRequest, "events are required")
	}
	seen := map[string]struct{}{}
	for _, name := range body.Events {
		if !knownEvent(name) {
			return apiErr(http.StatusBadRequest, "unknown event")
		}
		if _, ok := seen[name]; ok {
			return apiErr(http.StatusBadRequest, "duplicate event")
		}
		seen[name] = struct{}{}
	}
	return nil
}

func newSecret() (string, error) {
	raw, _, err := newToken()
	return raw, err
}

func (s *server) deliverWebhook(ctx context.Context, job dueJob) error {
	if job.SubscriptionID == "" {
		return fmt.Errorf("webhook job is missing its subscription")
	}
	var hookURL, secret string
	err := s.pool.QueryRow(ctx, `
		select url, secret
		from webhook_subscriptions
		where id = $1::uuid and organization_id = $2::uuid
	`, job.SubscriptionID, job.OrgID).Scan(&hookURL, &secret)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	event, err := s.loadWebhookEvent(ctx, job.EventID)
	if err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Openforce-Webhook")
	req.Header.Set("X-Openforce-Signature", signWebhook(secret, body))
	resp, err := webhookClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func signWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (s *server) loadWebhookEvent(ctx context.Context, id string) (webhookEvent, error) {
	var event webhookEvent
	var objectID, recordID sql.NullString
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select id::text, name, organization_id::text, actor_id::text,
			object_id::text, record_id::text, source, payload, created_at
		from events
		where id = $1::uuid
	`, id).Scan(&event.ID, &event.Name, &event.OrganizationID, &event.ActorID, &objectID, &recordID, &event.Source, &raw, &event.CreatedAt)
	if err != nil {
		return webhookEvent{}, err
	}
	event.ObjectID = nullText(objectID)
	event.RecordID = nullText(recordID)
	event.Payload = map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &event.Payload); err != nil {
			return webhookEvent{}, err
		}
	}
	return event, nil
}
