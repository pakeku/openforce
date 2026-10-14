package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type credentialInput struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Secret       string `json:"secret"`
	TokenURL     string `json:"token_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

type credentialPatch struct {
	Secret       *string `json:"secret"`
	ClientSecret *string `json:"client_secret"`
	RefreshToken *string `json:"refresh_token"`
}

type credentialView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Label     string     `json:"label"`
	Kind      string     `json:"kind"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (s *server) createCredential(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body credentialInput
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateCredential(body); err != nil {
		writeErr(w, err)
		return
	}
	view := credentialView{ID: uuid.NewString(), Name: body.Name, Label: body.Label, Kind: body.Kind}
	_, err := s.pool.Exec(r.Context(), `
		insert into credentials
			(id, organization_id, name, label, kind, secret, token_url, client_id, client_secret, refresh_token)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, view.ID, a.OrgID, view.Name, view.Label, view.Kind, body.Secret,
		nullIfEmpty(body.TokenURL), nullIfEmpty(body.ClientID), nullIfEmpty(body.ClientSecret), nullIfEmpty(body.RefreshToken))
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "credential already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *server) listCredentials(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		select id::text, name, label, kind, expires_at
		from credentials
		where organization_id = $1::uuid
		order by name
	`, a.OrgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	views := []credentialView{}
	for rows.Next() {
		view, err := scanCredential(rows)
		if err != nil {
			writeErr(w, err)
			return
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": views})
}

func (s *server) getCredential(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	view, err := s.loadCredential(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) patchCredential(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body credentialPatch
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	var kind string
	err := s.pool.QueryRow(r.Context(), `
		select kind from credentials
		where organization_id = $1::uuid and name = $2
	`, a.OrgID, r.PathValue("name")).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	switch kind {
	case "api_key":
		if body.Secret == nil || body.ClientSecret != nil || body.RefreshToken != nil || *body.Secret == "" {
			writeErr(w, apiErr(http.StatusBadRequest, "secret is required"))
			return
		}
		tag, err := s.pool.Exec(r.Context(), `
			update credentials
			set secret = $1, updated_at = now()
			where organization_id = $2::uuid and name = $3 and kind = 'api_key'
		`, *body.Secret, a.OrgID, r.PathValue("name"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if tag.RowsAffected() == 0 {
			writeErr(w, apiErr(http.StatusNotFound, "not found"))
			return
		}
	case "oauth":
		if body.Secret != nil || (body.ClientSecret == nil && body.RefreshToken == nil) {
			writeErr(w, apiErr(http.StatusBadRequest, "invalid credential"))
			return
		}
		if body.ClientSecret != nil && *body.ClientSecret == "" || body.RefreshToken != nil && *body.RefreshToken == "" {
			writeErr(w, apiErr(http.StatusBadRequest, "invalid credential"))
			return
		}
		var clientSecret, refreshToken any
		if body.ClientSecret != nil {
			clientSecret = *body.ClientSecret
		}
		if body.RefreshToken != nil {
			refreshToken = *body.RefreshToken
		}
		tag, err := s.pool.Exec(r.Context(), `
			update credentials
			set client_secret = coalesce($1, client_secret),
				refresh_token = coalesce($2, refresh_token),
				updated_at = now()
			where organization_id = $3::uuid and name = $4 and kind = 'oauth'
		`, clientSecret, refreshToken, a.OrgID, r.PathValue("name"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if tag.RowsAffected() == 0 {
			writeErr(w, apiErr(http.StatusNotFound, "not found"))
			return
		}
	default:
		writeErr(w, apiErr(http.StatusBadRequest, "invalid kind"))
		return
	}
	view, err := s.loadCredential(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) deleteCredential(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `
		delete from credentials
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

func (s *server) refreshCredential(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var kind, tokenURL, clientID, clientSecret, refreshToken string
	err := s.pool.QueryRow(r.Context(), `
		select kind, coalesce(token_url, ''), coalesce(client_id, ''),
			coalesce(client_secret, ''), coalesce(refresh_token, '')
		from credentials
		where organization_id = $1::uuid and name = $2
	`, a.OrgID, r.PathValue("name")).Scan(&kind, &tokenURL, &clientID, &clientSecret, &refreshToken)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if kind != "oauth" {
		writeErr(w, apiErr(http.StatusBadRequest, "invalid kind"))
		return
	}
	access, nextRefresh, expires, err := exchangeRefresh(r.Context(), tokenURL, clientID, clientSecret, refreshToken)
	if err != nil {
		writeErr(w, err)
		return
	}
	if nextRefresh == "" {
		nextRefresh = refreshToken
	}
	var expiresAt any
	if expires != nil {
		expiresAt = *expires
	}
	tag, err := s.pool.Exec(r.Context(), `
		update credentials
		set secret = $1, refresh_token = $2, expires_at = $3, updated_at = now()
		where organization_id = $4::uuid and name = $5 and kind = 'oauth'
	`, access, nextRefresh, expiresAt, a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, apiErr(http.StatusNotFound, "not found"))
		return
	}
	view, err := s.loadCredential(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func exchangeRefresh(ctx context.Context, tokenURL, clientID, clientSecret, refreshToken string) (string, string, *time.Time, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", nil, apiErr(http.StatusBadGateway, "refresh failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Openforce-Credential")
	resp, err := webhookClient.Do(req)
	if err != nil {
		return "", "", nil, apiErr(http.StatusBadGateway, "refresh failed")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", nil, apiErr(http.StatusBadGateway, "refresh failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", nil, apiErr(http.StatusBadGateway, "refresh failed")
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &token); err != nil || token.AccessToken == "" {
		return "", "", nil, apiErr(http.StatusBadGateway, "refresh failed")
	}
	var expires *time.Time
	if token.ExpiresIn > 0 {
		when := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
		expires = &when
	}
	return token.AccessToken, token.RefreshToken, expires, nil
}

func validateCredential(body credentialInput) error {
	if !namePattern.MatchString(body.Name) {
		return apiErr(http.StatusBadRequest, "invalid name")
	}
	if body.Label == "" {
		return apiErr(http.StatusBadRequest, "label is required")
	}
	switch body.Kind {
	case "api_key":
		if body.Secret == "" {
			return apiErr(http.StatusBadRequest, "secret is required")
		}
		if body.TokenURL != "" || body.ClientID != "" || body.ClientSecret != "" || body.RefreshToken != "" {
			return apiErr(http.StatusBadRequest, "invalid credential")
		}
	case "oauth":
		if err := validHTTPURL(body.TokenURL); err != nil {
			return err
		}
		if body.ClientID == "" {
			return apiErr(http.StatusBadRequest, "client id is required")
		}
		if body.ClientSecret == "" {
			return apiErr(http.StatusBadRequest, "client secret is required")
		}
		if body.RefreshToken == "" {
			return apiErr(http.StatusBadRequest, "refresh token is required")
		}
	default:
		return apiErr(http.StatusBadRequest, "invalid kind")
	}
	return nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *server) loadCredential(ctx context.Context, orgID, name string) (credentialView, error) {
	row := s.pool.QueryRow(ctx, `
		select id::text, name, label, kind, expires_at
		from credentials
		where organization_id = $1::uuid and name = $2
	`, orgID, name)
	view, err := scanCredential(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return credentialView{}, apiErr(http.StatusNotFound, "not found")
	}
	return view, err
}

func scanCredential(row pgx.Row) (credentialView, error) {
	var view credentialView
	var expires sql.NullTime
	err := row.Scan(&view.ID, &view.Name, &view.Label, &view.Kind, &expires)
	if expires.Valid {
		when := expires.Time
		view.ExpiresAt = &when
	}
	return view, err
}
