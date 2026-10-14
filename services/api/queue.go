package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const (
	jobStream   = "openforce:jobs"
	jobGroup    = "workers"
	jobConsumer = "work"
)

// sharedRedis is read by newMux. Tests set it before a request. An empty
// REDIS_URL leaves it nil, and the worker keeps polling Postgres.
var sharedRedis *redis.Client

func openRedis(url string) (*redis.Client, error) {
	if url == "" {
		return nil, nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return redis.NewClient(opt), nil
}

// committed commits the caller's transaction, then tells Redis about due jobs.
// A publish failure is logged. The caller's write has already committed.
func (s *server) committed(ctx context.Context, tx pgx.Tx) error {
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if err := s.publishDueJobs(ctx); err != nil {
		log.Println("publish jobs:", err)
	}
	return nil
}

func (s *server) publishDueJobs(ctx context.Context) error {
	if s.redis == nil {
		return nil
	}
	if err := ensureJobGroup(ctx, s.redis); err != nil {
		return err
	}
	for {
		claimed, err := s.claimDueJobs(ctx, 100)
		if err != nil {
			return err
		}
		if len(claimed) == 0 {
			return nil
		}
		for i, job := range claimed {
			err := s.redis.XAdd(ctx, &redis.XAddArgs{
				Stream: jobStream,
				Values: map[string]any{
					"job_id":          job.ID,
					"organization_id": job.OrgID,
				},
			}).Err()
			if err != nil {
				for _, left := range claimed[i:] {
					_ = s.unpublishJob(ctx, left.ID)
				}
				return err
			}
		}
	}
}

func (s *server) claimDueJobs(ctx context.Context, limit int) ([]dueJob, error) {
	rows, err := s.pool.Query(ctx, `
		with due as (
			select id from jobs
			where status = 'pending' and published_at is null and run_at <= now()
			order by created_at, id
			limit $1
			for update skip locked
		)
		update jobs j
		set published_at = now()
		from due
		where j.id = due.id
		returning j.id::text, j.organization_id::text
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []dueJob{}
	for rows.Next() {
		var job dueJob
		if err := rows.Scan(&job.ID, &job.OrgID); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *server) unpublishJob(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		update jobs set published_at = null
		where id = $1::uuid and status = 'pending'
	`, id)
	return err
}

// consumeQueuedJobs reads new stream messages once and does not wait.
// Postgres remains the record of status, attempts, and dead letters.
func (s *server) consumeQueuedJobs(ctx context.Context) (int, error) {
	if s.redis == nil {
		return 0, nil
	}
	if err := ensureJobGroup(ctx, s.redis); err != nil {
		return 0, err
	}
	delivered := 0
	for {
		streams, err := s.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    jobGroup,
			Consumer: jobConsumer,
			Streams:  []string{jobStream, ">"},
			Count:    100,
			// A negative block omits BLOCK. Zero sends BLOCK 0, which waits forever.
			Block: -1,
		}).Result()
		if errors.Is(err, redis.Nil) {
			return delivered, nil
		}
		if err != nil {
			return delivered, err
		}
		nmsgs := 0
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				nmsgs++
				ok, err := s.finishQueued(ctx, msg)
				if err != nil {
					return delivered, err
				}
				if ok {
					delivered++
				}
			}
		}
		if nmsgs == 0 {
			return delivered, nil
		}
	}
}

func (s *server) finishQueued(ctx context.Context, msg redis.XMessage) (bool, error) {
	id := streamString(msg.Values["job_id"])
	if _, err := uuid.Parse(id); err != nil {
		return false, s.ackJob(ctx, msg.ID)
	}
	job, ok, err := s.pendingJob(ctx, id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, s.ackJob(ctx, msg.ID)
	}
	done, err := s.completeJob(ctx, job)
	if err != nil {
		return false, err
	}
	if err := s.ackJob(ctx, msg.ID); err != nil {
		return false, err
	}
	return done, nil
}

func (s *server) pendingJob(ctx context.Context, id string) (dueJob, bool, error) {
	var job dueJob
	var subscriptionID, automationID sql.NullString
	err := s.pool.QueryRow(ctx, `
		select id::text, organization_id::text, event_id::text, kind, attempts, subscription_id::text, automation_id::text
		from jobs
		where id = $1::uuid and status = 'pending'
	`, id).Scan(&job.ID, &job.OrgID, &job.EventID, &job.Kind, &job.Attempts, &subscriptionID, &automationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dueJob{}, false, nil
	}
	if err != nil {
		return dueJob{}, false, err
	}
	if subscriptionID.Valid {
		job.SubscriptionID = subscriptionID.String
	}
	if automationID.Valid {
		job.AutomationID = automationID.String
	}
	return job, true, nil
}

func (s *server) ackJob(ctx context.Context, id string) error {
	return s.redis.XAck(ctx, jobStream, jobGroup, id).Err()
}

func ensureJobGroup(ctx context.Context, client *redis.Client) error {
	err := client.XGroupCreateMkStream(ctx, jobStream, jobGroup, "0").Err()
	if err == nil || (err != nil && strings.Contains(err.Error(), "BUSYGROUP")) {
		return nil
	}
	return err
}

func streamString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(t)
	}
}
