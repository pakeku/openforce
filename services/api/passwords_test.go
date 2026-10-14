package main

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestPasswordSignIn(t *testing.T) {
	t.Setenv("DATABASE_URL", testURL)
	ctx := context.Background()
	secret := "correct-horse"

	code, out := seedRun(t, "-org", "Password Plain", "-email", "password-plain@example.com", "-name", "Pat")
	if code != 0 {
		t.Fatalf("seed without password returned %d\n%s", code, out)
	}
	plainOrg, plainToken := seedFields(t, out)
	if strings.Contains(out, "password") {
		t.Fatalf("seed output %s", out)
	}
	var plainHash sql.NullString
	if err := testPool.QueryRow(ctx, `select password_hash from users where email = $1`, "password-plain@example.com").Scan(&plainHash); err != nil {
		t.Fatal(err)
	}
	if plainHash.Valid {
		t.Fatal("seed without -password stored a hash")
	}
	rec := apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "password-plain@example.com", "password": secret,
	})
	requireUnauthorized(t, rec)
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", plainToken, nil)
	requireStatus(t, rec, http.StatusOK)

	code, out = seedRun(t, "-org", "Password North", "-email", "Password.Owner@Example.com", "-name", "Nia", "-password", secret)
	if code != 0 {
		t.Fatalf("seed with password returned %d\n%s", code, out)
	}
	if strings.Contains(out, secret) || len(strings.Split(strings.TrimSpace(out), "\n")) != 2 {
		t.Fatalf("seed output %q", out)
	}
	orgID, seedToken := seedFields(t, out)
	var stored sql.NullString
	if err := testPool.QueryRow(ctx, `select password_hash from users where email = $1`, "password.owner@example.com").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !stored.Valid || stored.String == secret || bcrypt.CompareHashAndPassword([]byte(stored.String), []byte(secret)) != nil {
		t.Fatal("seed did not store a bcrypt hash")
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", seedToken, nil)
	requireStatus(t, rec, http.StatusOK)

	loginToken, loginOrg := loginAs(t, "Password.Owner@Example.com", secret, "")
	if loginOrg != orgID || loginToken == seedToken || loginToken == "" {
		t.Fatalf("login token %s org %s seed %s", loginToken, loginOrg, orgID)
	}
	if userOf(t, loginToken) != userOf(t, seedToken) {
		t.Fatal("login token is a different user")
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", loginToken, nil)
	requireStatus(t, rec, http.StatusOK)

	rec = apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "password.owner@example.com", "password": "wrong-password",
	})
	requireUnauthorized(t, rec)
	rec = apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "nobody@example.com", "password": secret,
	})
	requireUnauthorized(t, rec)

	other := newOwner(t, "Password South", "password-other@example.com")
	otherOrg := orgOf(t, other)
	if _, err := testPool.Exec(ctx, `
		insert into memberships (organization_id, user_id, role)
		values ($1::uuid, $2::uuid, 'member')
	`, otherOrg, userOf(t, seedToken)); err != nil {
		t.Fatal(err)
	}
	rec = apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "password.owner@example.com", "password": secret,
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "organization is required" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "password.owner@example.com", "password": "wrong-password",
	})
	requireUnauthorized(t, rec)

	northToken, northOrg := loginAs(t, "password.owner@example.com", secret, strings.ToUpper(orgID))
	if northOrg != orgID || orgOf(t, northToken) != orgID {
		t.Fatalf("north token org %s want %s", northOrg, orgID)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", northToken, nil)
	requireStatus(t, rec, http.StatusOK)
	southToken, southOrg := loginAs(t, "password.owner@example.com", secret, otherOrg)
	if southOrg != otherOrg || orgOf(t, southToken) != otherOrg || userOf(t, southToken) != userOf(t, seedToken) {
		t.Fatalf("south token org %s user mismatch", southOrg)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", southToken, nil)
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "password.owner@example.com", "password": secret, "organization": uuid.NewString(),
	})
	requireUnauthorized(t, rec)
	rec = apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "password.owner@example.com", "password": secret, "organization": "Password North",
	})
	requireUnauthorized(t, rec)

	rec = apiDo(t, http.MethodPost, "/api/members", "", map[string]any{
		"name": "Mia", "email": "password.member@example.com", "password": secret,
	})
	requireStatus(t, rec, http.StatusUnauthorized)
	rec = apiDo(t, http.MethodPost, "/api/members", seedToken, map[string]any{
		"name": "Mia", "email": "password.member@example.com", "password": "short",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "password is required" {
		t.Fatalf("short %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/members", seedToken, map[string]any{
		"name": "Mia", "email": "password.member@example.com",
	})
	requireStatus(t, rec, http.StatusBadRequest)
	if errorOf(t, rec) != "password is required" {
		t.Fatalf("missing %s", rec.Body.String())
	}
	var shortCount int
	if err := testPool.QueryRow(ctx, `select count(*) from users where email = $1`, "password.member@example.com").Scan(&shortCount); err != nil {
		t.Fatal(err)
	}
	if shortCount != 0 {
		t.Fatal("short password created a user")
	}

	rec = apiDo(t, http.MethodPost, "/api/members", seedToken, map[string]any{
		"name": "Mia", "email": "Password.Member@Example.com", "password": "12345678",
	})
	requireStatus(t, rec, http.StatusCreated)
	var created map[string]any
	mustJSON(t, rec, &created)
	if len(created) != 3 || created["email"] != "password.member@example.com" || created["name"] != "Mia" || created["role"] != "member" {
		t.Fatalf("member %#v", created)
	}
	if _, ok := created["token"]; ok {
		t.Fatalf("member response leaked a token %s", rec.Body.String())
	}
	if _, ok := created["password"]; ok || strings.Contains(rec.Body.String(), "12345678") {
		t.Fatalf("member response leaked the password %s", rec.Body.String())
	}
	var memberHash sql.NullString
	if err := testPool.QueryRow(ctx, `select password_hash from users where email = $1`, "password.member@example.com").Scan(&memberHash); err != nil {
		t.Fatal(err)
	}
	if !memberHash.Valid || bcrypt.CompareHashAndPassword([]byte(memberHash.String), []byte("12345678")) != nil {
		t.Fatal("member hash was not stored")
	}
	rec = apiDo(t, http.MethodPost, "/api/members", seedToken, map[string]any{
		"name": "Mia", "email": "password.member@example.com", "password": "12345678",
	})
	requireStatus(t, rec, http.StatusConflict)
	if errorOf(t, rec) != "email already exists" {
		t.Fatalf("duplicate %s", rec.Body.String())
	}
	rec = apiDo(t, http.MethodPost, "/api/members", other, map[string]any{
		"name": "Mia", "email": "Password.Member@Example.com", "password": "12345678",
	})
	requireStatus(t, rec, http.StatusConflict)

	memberToken, memberOrg := loginAs(t, "Password.Member@Example.com", "12345678", "")
	if memberOrg != orgID {
		t.Fatalf("member org %s want %s", memberOrg, orgID)
	}
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", memberToken, nil)
	requireStatus(t, rec, http.StatusOK)
	rec = apiDo(t, http.MethodPost, "/api/members", memberToken, map[string]any{
		"name": "Ned", "email": "password-ned@example.com", "password": "12345678",
	})
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("member create %s", rec.Body.String())
	}

	for _, ev := range eventsNamed(listEvents(t, seedToken, "name=user.created"), "user.created") {
		if _, ok := ev.Payload["password"]; ok {
			t.Fatalf("event leaked %+v", ev.Payload)
		}
		if _, ok := ev.Payload["password_hash"]; ok {
			t.Fatalf("event leaked %+v", ev.Payload)
		}
		if ev.Payload["email"] == secret || ev.Payload["name"] == secret {
			t.Fatalf("event leaked %+v", ev.Payload)
		}
	}

	rec = apiDo(t, http.MethodPost, "/api/keys", seedToken, map[string]any{
		"name": "svc", "label": "Service", "grants": []any{},
	})
	requireStatus(t, rec, http.StatusCreated)
	var key apiKeyView
	mustJSON(t, rec, &key)
	rec = apiDo(t, http.MethodPost, "/api/members", key.Token, map[string]any{
		"name": "Ned", "email": "password-ned@example.com", "password": "12345678",
	})
	requireStatus(t, rec, http.StatusForbidden)
	if strings.TrimSpace(rec.Body.String()) != `{"error":"forbidden"}` {
		t.Fatalf("service create %s", rec.Body.String())
	}

	added, err := (&server{pool: testPool}).addMember(ctx, orgID, "Ada", "password-added@example.com")
	if err != nil {
		t.Fatal(err)
	}
	var addedHash sql.NullString
	if err := testPool.QueryRow(ctx, `select password_hash from users where email = $1`, "password-added@example.com").Scan(&addedHash); err != nil {
		t.Fatal(err)
	}
	if addedHash.Valid {
		t.Fatal("addMember stored a password")
	}
	rec = apiDo(t, http.MethodPost, "/api/login", "", map[string]any{
		"email": "password-added@example.com", "password": secret,
	})
	requireUnauthorized(t, rec)
	rec = apiDo(t, http.MethodGet, "/api/meta/objects", added, nil)
	requireStatus(t, rec, http.StatusOK)

	var due int
	if err := testPool.QueryRow(ctx, `
		select count(*) from jobs
		where status = 'pending' and run_at <= now()
		  and organization_id in ($1::uuid, $2::uuid, $3::uuid)
	`, orgID, plainOrg, otherOrg).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if due != 0 {
		t.Fatalf("left %d due jobs", due)
	}
}

func loginAs(t *testing.T, email, password, org string) (string, string) {
	t.Helper()
	body := map[string]any{"email": email, "password": password}
	if org != "" {
		body["organization"] = org
	}
	rec := apiDo(t, http.MethodPost, "/api/login", "", body)
	requireStatus(t, rec, http.StatusOK)
	var session map[string]any
	mustJSON(t, rec, &session)
	token, _ := session["token"].(string)
	organization, _ := session["organization"].(string)
	if len(session) != 2 || token == "" || organization == "" {
		t.Fatalf("session %#v", session)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("login leaked %s", rec.Body.String())
	}
	return token, organization
}

func requireUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	requireStatus(t, rec, http.StatusUnauthorized)
	if errorOf(t, rec) != "unauthorized" {
		t.Fatalf("body %s", rec.Body.String())
	}
	lower := strings.ToLower(rec.Body.String())
	if strings.Contains(lower, "password") || strings.Contains(lower, "email") || strings.Contains(lower, "hash") {
		t.Fatalf("body named a field %s", rec.Body.String())
	}
}

func seedFields(t *testing.T, out string) (string, string) {
	t.Helper()
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if ok {
			fields[key] = strings.TrimSpace(value)
		}
	}
	if fields["organization"] == "" || fields["token"] == "" {
		t.Fatalf("seed output %s", out)
	}
	return fields["organization"], fields["token"]
}

func seedRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := runSeed(args)
	w.Close()
	os.Stdout = old
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(raw)
}
