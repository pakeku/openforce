package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type actor struct {
	UserID string
	OrgID  string
	Role   string
}

type actorKeyType struct{}

func actorFrom(ctx context.Context) actor {
	a, _ := ctx.Value(actorKeyType{}).(actor)
	return a
}

func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.authenticate(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), actorKeyType{}, a)))
	}
}

func (s *server) authenticate(r *http.Request) (actor, error) {
	raw, err := bearerToken(r.Header.Get("Authorization"))
	if err != nil {
		return actor{}, err
	}
	var a actor
	err = s.pool.QueryRow(r.Context(), `
		select t.user_id::text, t.organization_id::text, m.role
		from access_tokens t
		join memberships m
		  on m.user_id = t.user_id
		 and m.organization_id = t.organization_id
		where t.token_hash = $1
	`, tokenHash(raw)).Scan(&a.UserID, &a.OrgID, &a.Role)
	if err == pgx.ErrNoRows {
		return actor{}, apiErr(http.StatusUnauthorized, "unauthorized")
	}
	if err != nil {
		return actor{}, err
	}
	return a, nil
}

func bearerToken(header string) (string, error) {
	scheme, raw, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return "", apiErr(http.StatusUnauthorized, "unauthorized")
	}
	return raw, nil
}

func newToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	return raw, tokenHash(raw), nil
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *server) bootstrap(ctx context.Context, orgName, userName, email, role string) (orgID, token string, err error) {
	if role != "owner" && role != "member" {
		return "", "", apiErr(http.StatusBadRequest, "invalid role")
	}
	raw, hash, err := newToken()
	if err != nil {
		return "", "", err
	}
	orgID = uuid.NewString()
	userID := uuid.NewString()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `insert into organizations (id, name) values ($1, $2)`, orgID, orgName); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, `insert into users (id, email, name) values ($1, $2, $3)`, userID, strings.ToLower(email), userName); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, `insert into memberships (organization_id, user_id, role) values ($1, $2, $3)`, orgID, userID, role); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, `
		insert into access_tokens (id, organization_id, user_id, token_hash)
		values ($1, $2, $3, $4)
	`, uuid.NewString(), orgID, userID, hash); err != nil {
		return "", "", err
	}
	if err = insertEvent(ctx, tx, orgID, eventWrite{
		Name:    "user.created",
		ActorID: userID,
		Source:  sourceAdmin,
		Payload: map[string]any{"user_id": userID, "email": strings.ToLower(email), "name": userName},
	}); err != nil {
		return "", "", err
	}
	if err = s.committed(ctx, tx); err != nil {
		return "", "", err
	}
	return orgID, raw, nil
}

func (s *server) addMember(ctx context.Context, orgID, userName, email string) (string, error) {
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	userID := uuid.NewString()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var actorID string
	if err = tx.QueryRow(ctx, `
		select user_id::text from memberships
		where organization_id = $1::uuid and role = 'owner'
		order by user_id
		limit 1
	`, orgID).Scan(&actorID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `insert into users (id, email, name) values ($1, $2, $3)`, userID, strings.ToLower(email), userName); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `insert into memberships (organization_id, user_id, role) values ($1, $2, 'member')`, orgID, userID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `
		insert into access_tokens (id, organization_id, user_id, token_hash)
		values ($1, $2, $3, $4)
	`, uuid.NewString(), orgID, userID, hash); err != nil {
		return "", err
	}
	if err = insertEvent(ctx, tx, orgID, eventWrite{
		Name:    "user.created",
		ActorID: actorID,
		Source:  sourceAdmin,
		Payload: map[string]any{"user_id": userID, "email": strings.ToLower(email), "name": userName},
	}); err != nil {
		return "", err
	}
	if err = s.committed(ctx, tx); err != nil {
		return "", err
	}
	return raw, nil
}
