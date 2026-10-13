package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationFilesAreSortedSQLOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "0002_b.sql"), "select 2;")
	writeFile(t, filepath.Join(dir, "0001_a.sql"), "select 1;")
	writeFile(t, filepath.Join(dir, "notes.txt"), "ignore")

	got, err := migrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"0001_a.sql", "0002_b.sql"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func TestSelectPendingPreservesOrder(t *testing.T) {
	applied := map[string]struct{}{"0001_a.sql": {}}
	got := selectPending([]string{"0001_a.sql", "0002_b.sql", "0003_c.sql"}, applied)
	want := []string{"0002_b.sql", "0003_c.sql"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestFindMigrationsFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	migrations := filepath.Join(root, "db", "migrations")
	if err := os.MkdirAll(migrations, 0o755); err != nil {
		t.Fatal(err)
	}
	start := filepath.Join(root, "services", "api")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findMigrationsFrom(start)
	if err != nil {
		t.Fatal(err)
	}
	if got != migrations {
		t.Fatalf("got %s, want %s", got, migrations)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
