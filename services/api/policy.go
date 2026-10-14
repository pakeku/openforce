package main

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
)

type permissionBody struct {
	Role     string `json:"role"`
	CanRead  bool   `json:"can_read"`
	CanWrite bool   `json:"can_write"`
}

func (s *server) putPermissions(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	if a.Role != "owner" {
		writeErr(w, apiErr(http.StatusForbidden, "forbidden"))
		return
	}
	objectID, err := s.objectByName(r.Context(), a.OrgID, r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body permissionBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Role != "member" {
		writeErr(w, apiErr(http.StatusBadRequest, "only the member role can be granted"))
		return
	}
	_, err = s.pool.Exec(r.Context(), `
		insert into object_permissions (organization_id, role, object_id, can_read, can_write)
		values ($1, $2, $3, $4, $5)
		on conflict (organization_id, role, object_id)
		do update set can_read = excluded.can_read, can_write = excluded.can_write
	`, a.OrgID, body.Role, objectID, body.CanRead, body.CanWrite)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"role":      body.Role,
		"can_read":  body.CanRead,
		"can_write": body.CanWrite,
	})
}

func (s *server) authorize(ctx context.Context, a actor, objectID string, write bool) error {
	if a.Role == "owner" {
		return nil
	}
	if a.Role == "service" {
		return s.authorizeKey(ctx, a, objectID, write)
	}
	if a.Role != "member" {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	var canRead, canWrite bool
	err := s.pool.QueryRow(ctx, `
		select can_read, can_write
		from object_permissions
		where organization_id = $1 and role = 'member' and object_id = $2
	`, a.OrgID, objectID).Scan(&canRead, &canWrite)
	if err == pgx.ErrNoRows {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	if err != nil {
		return err
	}
	if write && !canWrite {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	if !write && !canRead {
		return apiErr(http.StatusForbidden, "forbidden")
	}
	return nil
}
