package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestOpenRedisReadsTheURL(t *testing.T) {
	client, err := openRedis("")
	if err != nil || client != nil {
		t.Fatalf("empty url client=%v err=%v", client, err)
	}
	if _, err := openRedis("not-a-url"); err == nil {
		t.Fatal("accepted a url redis cannot parse")
	}
}

func TestRedisWakesTheWorker(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	client, err := openRedis("redis://" + mr.Addr() + "/0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	sharedRedis = client
	t.Cleanup(func() { sharedRedis = nil })

	token := newOwner(t, "Queue", "queue-owner@example.com")
	postObject(t, token, companyObject())
	acme := postRecord(t, token, "company", map[string]any{"name": "Queued", "employees": 4, "industry": "SaaS"})
	if len(listAudit(t, token, acme.ID)) != 0 {
		t.Fatal("the record request wrote audit entries")
	}

	orgID := orgOf(t, token)
	ids := fieldJobIDs(t, acme.ID)
	if len(ids) != 3 {
		t.Fatalf("field jobs %v", ids)
	}
	messages, err := client.XRange(context.Background(), jobStream, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, msg := range messages {
		seen[streamString(msg.Values["job_id"])] = streamString(msg.Values["organization_id"])
	}
	for _, id := range ids {
		if seen[id] != orgID {
			t.Fatalf("job %s organization %q", id, seen[id])
		}
		var published bool
		if err := testPool.QueryRow(context.Background(), `
			select published_at is not null from jobs where id = $1::uuid
		`, id).Scan(&published); err != nil {
			t.Fatal(err)
		}
		if !published {
			t.Fatalf("job %s was not marked published", id)
		}
	}

	worker := &server{pool: testPool, redis: client}
	n, err := worker.consumeQueuedJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n < len(ids) {
		t.Fatalf("consumed %d", n)
	}
	entries := listAudit(t, token, acme.ID)
	if len(entries) != 3 || entries[0].After != "Queued" {
		t.Fatalf("entries %#v", entries)
	}
	for _, id := range ids {
		var status string
		if err := testPool.QueryRow(context.Background(), `
			select status from jobs where id = $1::uuid
		`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "done" {
			t.Fatalf("job %s status %s", id, status)
		}
	}
	again, err := worker.consumeQueuedJobs(context.Background())
	if err != nil || again != 0 {
		t.Fatalf("second consume %d %v", again, err)
	}
}

func fieldJobIDs(t *testing.T, recordID string) []string {
	t.Helper()
	rows, err := testPool.Query(context.Background(), `
		select j.id::text
		from jobs j
		join events e on e.id = j.event_id
		where e.record_id = $1::uuid and e.name = 'field.changed' and j.kind = 'audit'
		order by e.seq
	`, recordID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}
