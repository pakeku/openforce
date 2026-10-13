package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	testURL  string
	testPool *pgxpool.Pool
)

func TestMain(m *testing.M) {
	cleanup, err := startTestDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := 1
	defer func() {
		cleanup()
		os.Exit(code)
	}()
	code = m.Run()
}

func startTestDB() (func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	dir, err := os.MkdirTemp("", "openforce-pg-")
	if err != nil {
		return nil, err
	}
	logs := &lockedBuf{}
	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(port).
		Database("openforce").
		Username("openforce").
		Password("openforce").
		RuntimePath(dir).
		Locale("C").
		Encoding("UTF8").
		StartTimeout(5 * time.Minute).
		Logger(logs))
	cleanup := func() {
		if testPool != nil {
			testPool.Close()
		}
		if err := db.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "stop postgres:", err)
		}
		_ = os.RemoveAll(dir)
	}
	if err := db.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("start postgres: %w\n%s", err, logs.String())
	}

	testURL = fmt.Sprintf("postgres://openforce:openforce@127.0.0.1:%d/openforce?sslmode=disable", port)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testURL)
	if err != nil {
		cleanup()
		return nil, err
	}
	migrations, err := findMigrationsFrom(mustWD())
	if err != nil {
		conn.Close(ctx)
		cleanup()
		return nil, err
	}
	if _, err := applyMigrations(ctx, conn, migrations); err != nil {
		conn.Close(ctx)
		cleanup()
		return nil, err
	}
	conn.Close(ctx)

	testPool, err = openPool(ctx, testURL)
	if err != nil {
		cleanup()
		return nil, err
	}
	return cleanup, nil
}

func TestMigrationsApplied(t *testing.T) {
	ctx := context.Background()
	var n int
	if err := testPool.QueryRow(ctx, `select count(*) from schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("applied %d migrations, want 6", n)
	}

	conn, err := pgx.Connect(ctx, testURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	dir, err := findMigrationsFrom(mustWD())
	if err != nil {
		t.Fatal(err)
	}
	applied, err := applyMigrations(ctx, conn, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("second apply pending: %v", applied)
	}
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
