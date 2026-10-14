package main

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type loginBody struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Organization string `json:"organization"`
}

type sessionView struct {
	Token        string `json:"token"`
	Organization string `json:"organization"`
}

type memberBody struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type memberView struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func passwordHashArg(password string) (any, error) {
	if password == "" {
		return nil, nil
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return string(hashed), nil
}

// login is public. Unknown email, a wrong password, and a null hash share one 401.
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	var userID string
	var hash sql.NullString
	err := s.pool.QueryRow(r.Context(), `
		select id::text, password_hash from users where email = $1
	`, email).Scan(&userID, &hash)
	if err == pgx.ErrNoRows {
		writeErr(w, apiErr(http.StatusUnauthorized, "unauthorized"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if !hash.Valid || bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(body.Password)) != nil {
		writeErr(w, apiErr(http.StatusUnauthorized, "unauthorized"))
		return
	}
	orgs, err := s.organizationIDs(r.Context(), userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	orgID, err := chooseOrganization(orgs, body.Organization)
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, err := s.issueToken(r.Context(), orgID, userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionView{Token: raw, Organization: orgID})
}

func chooseOrganization(orgs []string, requested string) (string, error) {
	if len(orgs) == 1 {
		return orgs[0], nil
	}
	if len(orgs) == 0 {
		return "", apiErr(http.StatusUnauthorized, "unauthorized")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", apiErr(http.StatusBadRequest, "organization is required")
	}
	parsed, err := uuid.Parse(requested)
	if err != nil {
		return "", apiErr(http.StatusUnauthorized, "unauthorized")
	}
	want := parsed.String()
	for _, id := range orgs {
		if id == want {
			return id, nil
		}
	}
	return "", apiErr(http.StatusUnauthorized, "unauthorized")
}

func (s *server) organizationIDs(ctx context.Context, userID string) ([]string, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, apiErr(http.StatusUnauthorized, "unauthorized")
	}
	rows, err := s.pool.Query(ctx, `
		select organization_id::text
		from memberships
		where user_id = $1::uuid
		order by organization_id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *server) issueToken(ctx context.Context, orgID, userID string) (string, error) {
	if _, err := uuid.Parse(orgID); err != nil {
		return "", apiErr(http.StatusUnauthorized, "unauthorized")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return "", apiErr(http.StatusUnauthorized, "unauthorized")
	}
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		insert into access_tokens (id, organization_id, user_id, token_hash)
		values ($1, $2, $3, $4)
	`, uuid.NewString(), orgID, userID, hash)
	if err != nil {
		return "", err
	}
	return raw, nil
}

func (s *server) createMember(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if err := ownerOnly(a); err != nil {
		writeErr(w, err)
		return
	}
	var body memberBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if name == "" {
		writeErr(w, apiErr(http.StatusBadRequest, "name is required"))
		return
	}
	if email == "" {
		writeErr(w, apiErr(http.StatusBadRequest, "email is required"))
		return
	}
	if utf8.RuneCountInString(body.Password) < 8 {
		writeErr(w, apiErr(http.StatusBadRequest, "password is required"))
		return
	}
	passwordHash, err := passwordHashArg(body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	userID := uuid.NewString()
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `
		insert into users (id, email, name, password_hash) values ($1, $2, $3, $4)
	`, userID, email, name, passwordHash)
	if isUnique(err) {
		writeErr(w, apiErr(http.StatusConflict, "email already exists"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		insert into memberships (organization_id, user_id, role) values ($1, $2, 'member')
	`, a.OrgID, userID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = insertEvent(r.Context(), tx, a.OrgID, eventWrite{
		Name:    "user.created",
		ActorID: a.UserID,
		Source:  sourceAdmin,
		Payload: map[string]any{"user_id": userID, "email": email, "name": name},
	}); err != nil {
		writeErr(w, err)
		return
	}
	if err = s.committed(r.Context(), tx); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, memberView{Email: email, Name: name, Role: "member"})
}
