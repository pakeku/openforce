package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCredentialsStaySecret(t *testing.T) {
	var mu sync.Mutex
	var seen []url.Values
	var failNext bool
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		mu.Lock()
		seen = append(seen, form)
		fail := failNext
		failNext = false
		mu.Unlock()
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.Header.Get("User-Agent") != "Openforce-Credential" {
			http.Error(w, "nope", http.StatusBadRequest)
			return
		}
		if fail {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access-2","refresh_token":"refresh-2","expires_in":3600}`)
	}))
	defer tokenServer.Close()

	token := newOwner(t, "Credentials", "credentials-owner@example.com")
	orgID := orgOf(t, token)
	member, err := (&server{pool: testPool}).addMember(context.Background(), orgID, "Mia", "credentials-member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	rec := apiDo(t, http.MethodPost, "/api/credentials", member, map[string]any{
		"name": "stripe", "label": "Stripe", "kind": "api_key", "secret": "sk_live_123",
	})
	requireStatus(t, rec, http.StatusForbidden)
	rec = apiDo(t, http.MethodGet, "/api/credentials", "", nil)
	requireStatus(t, rec, http.StatusUnauthorized)
	rec = apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "Bad-Name", "label": "Stripe", "kind": "api_key", "secret": "sk_live_123",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid name" {
		t.Fatalf("name %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "stripe", "label": "Stripe", "kind": "basic", "secret": "sk_live_123",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid kind" {
		t.Fatalf("kind %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "stripe", "label": "Stripe", "kind": "api_key",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "secret is required" {
		t.Fatalf("secret %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "hub", "label": "Hub", "kind": "oauth", "token_url": "ftp://example.com/token",
		"client_id": "id", "client_secret": "cs", "refresh_token": "refresh-1",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid url" {
		t.Fatalf("url %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "stripe", "label": "Stripe", "kind": "api_key", "secret": "sk_live_123",
	})
	requireStatus(t, rec, http.StatusCreated)
	if strings.Contains(rec.Body.String(), "sk_live_123") {
		t.Fatalf("create returned the secret %s", rec.Body.String())
	}
	var created credentialView
	mustJSON(t, rec, &created)
	if created.Name != "stripe" || created.Kind != "api_key" || created.ExpiresAt != nil {
		t.Fatalf("created %+v", created)
	}
	if got := storedSecret(t, orgID, "stripe"); got != "sk_live_123" {
		t.Fatalf("stored %s", got)
	}
	rec = apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "stripe", "label": "Stripe", "kind": "api_key", "secret": "sk_live_123",
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "credential already exists" {
		t.Fatalf("duplicate %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/credentials", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "sk_live_123") {
		t.Fatalf("list returned the secret %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodGet, "/api/credentials/stripe", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "sk_live") {
		t.Fatalf("get returned the secret %s", rec.Body.String())
	}
	other := newOwner(t, "Credentials Other", "credentials-other@example.com")
	rec = apiDo(t, http.MethodGet, "/api/credentials/stripe", other, nil)
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodPatch, "/api/credentials/stripe", token, map[string]any{"secret": "sk_live_456"})
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "sk_live_456") || storedSecret(t, orgID, "stripe") != "sk_live_456" {
		t.Fatalf("rotation body %s stored %s", rec.Body.String(), storedSecret(t, orgID, "stripe"))
	}
	rec = apiDo(t, http.MethodPost, "/api/credentials/stripe/refresh", token, nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "invalid kind" {
		t.Fatalf("refresh key %s", rec.Body.String())
	}

	rec = apiDo(t, http.MethodPost, "/api/credentials", token, map[string]any{
		"name": "hub", "label": "Hub", "kind": "oauth", "token_url": tokenServer.URL,
		"client_id": "client-1", "client_secret": "secret-1", "refresh_token": "refresh-1",
	})
	requireStatus(t, rec, http.StatusCreated)
	if strings.Contains(rec.Body.String(), "secret-1") || strings.Contains(rec.Body.String(), "refresh-1") {
		t.Fatalf("oauth create leaked %s", rec.Body.String())
	}
	mu.Lock()
	failNext = true
	mu.Unlock()
	rec = apiDo(t, http.MethodPost, "/api/credentials/hub/refresh", token, nil)
	requireStatus(t, rec, http.StatusBadGateway)
	if errorOf(t, rec) != "refresh failed" {
		t.Fatalf("failed refresh %s", rec.Body.String())
	}
	if storedSecret(t, orgID, "hub") != "" || storedColumn(t, orgID, "hub", "refresh_token") != "refresh-1" {
		t.Fatal("a failed refresh changed the credential")
	}
	rec = apiDo(t, http.MethodPost, "/api/credentials/hub/refresh", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "access-2") || strings.Contains(rec.Body.String(), "refresh-2") || strings.Contains(rec.Body.String(), "secret-1") {
		t.Fatalf("refresh leaked %s", rec.Body.String())
	}
	var refreshed credentialView
	mustJSON(t, rec, &refreshed)
	if refreshed.ExpiresAt == nil || !refreshed.ExpiresAt.After(time.Now()) || refreshed.ExpiresAt.After(time.Now().Add(2*time.Hour)) {
		t.Fatalf("expiry %+v", refreshed.ExpiresAt)
	}
	if storedSecret(t, orgID, "hub") != "access-2" || storedColumn(t, orgID, "hub", "refresh_token") != "refresh-2" {
		t.Fatal("refresh did not store the new tokens")
	}
	rec = apiDo(t, http.MethodGet, "/api/credentials/hub", token, nil)
	requireStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), "access-2") || strings.Contains(rec.Body.String(), "refresh-2") {
		t.Fatalf("get leaked %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/credentials/hub/refresh", token, nil)
	requireStatus(t, rec, http.StatusOK)
	mu.Lock()
	calls := append([]url.Values{}, seen...)
	mu.Unlock()
	if len(calls) != 3 || calls[1].Get("refresh_token") != "refresh-1" || calls[1].Get("client_id") != "client-1" || calls[1].Get("client_secret") != "secret-1" || calls[1].Get("grant_type") != "refresh_token" || calls[2].Get("refresh_token") != "refresh-2" {
		t.Fatalf("token requests %+v", calls)
	}

	rec = apiDo(t, http.MethodDelete, "/api/credentials/hub", other, nil)
	requireStatus(t, rec, http.StatusNotFound)
	rec = apiDo(t, http.MethodDelete, "/api/credentials/stripe", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodDelete, "/api/credentials/hub", token, nil)
	requireStatus(t, rec, http.StatusNoContent)
	rec = apiDo(t, http.MethodGet, "/api/credentials/stripe", token, nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func storedSecret(t *testing.T, orgID, name string) string {
	t.Helper()
	return storedColumn(t, orgID, name, "secret")
}

func storedColumn(t *testing.T, orgID, name, column string) string {
	t.Helper()
	var value string
	err := testPool.QueryRow(context.Background(), `
		select `+column+` from credentials
		where organization_id = $1::uuid and name = $2
	`, orgID, name).Scan(&value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
