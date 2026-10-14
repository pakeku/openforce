package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxJobAttempts = 5

type dueJob struct {
	ID       string
	OrgID    string
	EventID  string
	Kind     string
	Attempts int
}

type jobView struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Attempts  int       `json:"attempts"`
	LastError *string   `json:"last_error"`
	RunAt     time.Time `json:"run_at"`
	CreatedAt time.Time `json:"created_at"`
}

func enqueueJob(ctx context.Context, tx pgx.Tx, orgID, eventID, kind string) error {
	_, err := tx.Exec(ctx, `
		insert into jobs (id, organization_id, event_id, kind)
		values ($1, $2, $3, $4)
	`, uuid.NewString(), orgID, eventID, kind)
	return err
}

// deliverJobs completes due jobs. Request handlers do not call it.
func (s *server) deliverJobs(ctx context.Context) (int, error) {
	delivered := 0
	for {
		n, scanned, err := s.deliverJobBatch(ctx, 100)
		delivered += n
		if err != nil || scanned == 0 {
			return delivered, err
		}
	}
}

func (s *server) deliverJobBatch(ctx context.Context, limit int) (int, int, error) {
	rows, err := s.pool.Query(ctx, `
		select id::text, organization_id::text, event_id::text, kind, attempts
		from jobs
		where status = 'pending' and run_at <= now()
		order by created_at, id
		limit $1
	`, limit)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	jobs := []dueJob{}
	for rows.Next() {
		var job dueJob
		if err := rows.Scan(&job.ID, &job.OrgID, &job.EventID, &job.Kind, &job.Attempts); err != nil {
			return 0, 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	delivered := 0
	for _, job := range jobs {
		ok, err := s.completeJob(ctx, job)
		if err != nil {
			return delivered, len(jobs), err
		}
		if ok {
			delivered++
		}
	}
	return delivered, len(jobs), nil
}

func (s *server) completeJob(ctx context.Context, job dueJob) (bool, error) {
	err := s.runJob(ctx, job)
	if err != nil {
		return false, s.failJob(ctx, job, err)
	}
	return s.finishJob(ctx, job)
}

func (s *server) runJob(ctx context.Context, job dueJob) error {
	switch job.Kind {
	case "audit":
		ev, err := s.loadAuditEvent(ctx, job.EventID)
		if err != nil {
			return err
		}
		_, err = s.insertAudit(ctx, ev)
		return err
	default:
		return fmt.Errorf("unknown job kind %s", job.Kind)
	}
}

func (s *server) finishJob(ctx context.Context, job dueJob) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		update jobs
		set status = 'done', attempts = attempts + 1, last_error = null, updated_at = now()
		where id = $1::uuid and status = 'pending'
	`, job.ID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *server) failJob(ctx context.Context, job dueJob, cause error) error {
	next := job.Attempts + 1
	status := "pending"
	runAt := time.Now().Add(jobBackoff(next))
	if next >= maxJobAttempts {
		status = "dead"
		runAt = time.Now()
	}
	_, err := s.pool.Exec(ctx, `
		update jobs
		set status = $2, attempts = $3, last_error = $4, run_at = $5, updated_at = now()
		where id = $1::uuid and status = 'pending'
	`, job.ID, status, next, trimError(cause), runAt)
	return err
}

func jobBackoff(attempts int) time.Duration {
	if attempts < 1 {
		return time.Second
	}
	return time.Second << (attempts - 1)
}

func trimError(err error) string {
	msg := err.Error()
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}

func (s *server) listJobs(w http.ResponseWriter, r *http.Request) {
	a := actorFrom(r.Context())
	status := r.URL.Query().Get("status")
	switch status {
	case "", "pending", "done", "dead":
	default:
		writeErr(w, apiErr(http.StatusBadRequest, "invalid status"))
		return
	}
	eventID := r.URL.Query().Get("event")
	if eventID != "" {
		if _, err := uuid.Parse(eventID); err != nil {
			writeErr(w, apiErr(http.StatusBadRequest, "invalid event"))
			return
		}
	}
	limit, err := listLimit(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	jobs, err := s.selectJobs(r.Context(), a.OrgID, status, eventID, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *server) selectJobs(ctx context.Context, orgID, status, eventID string, limit int) ([]jobView, error) {
	args := []any{orgID}
	query := `
		select id::text, event_id::text, kind, status, attempts, last_error, run_at, created_at
		from jobs
		where organization_id = $1::uuid`
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" and status = $%d", len(args))
	}
	if eventID != "" {
		args = append(args, eventID)
		query += fmt.Sprintf(" and event_id = $%d::uuid", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" order by created_at, id limit $%d", len(args))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []jobView{}
	for rows.Next() {
		var view jobView
		var last sql.NullString
		if err := rows.Scan(&view.ID, &view.EventID, &view.Kind, &view.Status, &view.Attempts, &last, &view.RunAt, &view.CreatedAt); err != nil {
			return nil, err
		}
		view.LastError = nullText(last)
		jobs = append(jobs, view)
	}
	return jobs, rows.Err()
}
