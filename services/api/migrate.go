package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"
)

func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func selectPending(files []string, applied map[string]struct{}) []string {
	var pending []string
	for _, name := range files {
		if _, ok := applied[name]; ok {
			continue
		}
		pending = append(pending, name)
	}
	return pending
}

func findMigrationsFrom(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, "db", "migrations")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("db/migrations not found from %s", start)
		}
		dir = parent
	}
}

func applyMigrations(ctx context.Context, conn *pgx.Conn, dir string) ([]string, error) {
	if _, err := conn.Exec(ctx, `
		create table if not exists schema_migrations (
			filename text primary key,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, `select filename from schema_migrations`)
	if err != nil {
		return nil, err
	}
	applied := map[string]struct{}{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		applied[name] = struct{}{}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	files, err := migrationFiles(dir)
	if err != nil {
		return nil, err
	}

	var done []string
	for _, name := range selectPending(files, applied) {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return done, err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return done, err
		}
		if err := execStatements(ctx, tx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return done, fmt.Errorf("%s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations (filename) values ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return done, fmt.Errorf("%s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return done, err
		}
		done = append(done, name)
	}
	return done, nil
}

func execStatements(ctx context.Context, tx pgx.Tx, body string) error {
	for _, stmt := range splitSQL(body) {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
