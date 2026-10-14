package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupFlagsMigrateSeedAndStart(t *testing.T) {
	root := fakeRepo(t)
	var calls []string
	env := &setupEnv{
		startDB: func(got string) (string, func(), error) {
			if got != root {
				t.Fatalf("root %s", got)
			}
			calls = append(calls, "db")
			return "postgres://openforce:openforce@127.0.0.1:5432/openforce?sslmode=disable", func() {}, nil
		},
		run: func(dir string, extra []string, name string, args ...string) error {
			calls = append(calls, name+" "+strings.Join(args, " "))
			if !strings.Contains(strings.Join(extra, " "), "DATABASE_URL=postgres://openforce:openforce@127.0.0.1:5432/openforce") {
				t.Fatalf("env %v", extra)
			}
			return nil
		},
		start: func(dir string, extra []string, name string, args ...string) (func(), error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return func() {}, nil
		},
		wait: func(url string) error { return nil },
	}
	var stdout, stderr strings.Builder
	done := make(chan struct{})
	close(done)
	code := setup([]string{"-org", "Acme", "-email", "ada@example.com", "-name", "Ada", "-password", "longpassword"}, strings.NewReader(""), &stdout, &stderr, root, done, env)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	if strings.Join(calls, "\n") != strings.Join([]string{
		"db",
		"go run . migrate",
		"go run . seed -org Acme -email ada@example.com -name Ada -password longpassword",
		"go run .",
		"npm run dev -- --host 127.0.0.1 --port 5173 --strictPort",
	}, "\n") {
		t.Fatalf("calls %v", calls)
	}
	if !strings.Contains(stdout.String(), "API http://127.0.0.1:8080\n") || !strings.Contains(stdout.String(), "Sign in as ada@example.com\n") {
		t.Fatalf("stdout %q", stdout.String())
	}
	raw, err := os.ReadFile(filepath.Join(root, ".openforce", "dev.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "longpassword") || strings.Contains(string(raw), "password") {
		t.Fatalf("config stored a password: %s", raw)
	}
}

func TestSetupReuseSkipsSeed(t *testing.T) {
	root := fakeRepo(t)
	if err := writeConfig(root, devConfig{Org: "Acme", Email: "ada@example.com", Name: "Ada", Port: 8080, Web: 5173, Database: "embedded"}); err != nil {
		t.Fatal(err)
	}
	var calls []string
	env := recordingEnv(t, &calls)
	done := make(chan struct{})
	close(done)
	var stderr strings.Builder
	code := setup([]string{"-reuse"}, strings.NewReader(""), ioDiscard(), &stderr, root, done, env)
	if code != 0 {
		t.Fatalf("exit %d %s", code, stderr.String())
	}
	for _, call := range calls {
		if strings.Contains(call, "seed") {
			t.Fatalf("seeded on reuse: %v", calls)
		}
	}
}

func TestSetupExistingOwnerContinues(t *testing.T) {
	root := fakeRepo(t)
	env := &setupEnv{
		startDB: func(string) (string, func(), error) {
			return "postgres://openforce:openforce@127.0.0.1:1/openforce?sslmode=disable", func() {}, nil
		},
		run: func(dir string, extra []string, name string, args ...string) error {
			if len(args) > 2 && args[2] == "seed" {
				return errString("duplicate key value violates unique constraint users_email_key")
			}
			return nil
		},
		start: func(string, []string, string, ...string) (func(), error) { return func() {}, nil },
		wait:  func(string) error { return nil },
	}
	done := make(chan struct{})
	close(done)
	var stderr strings.Builder
	code := setup([]string{"-org", "Acme", "-email", "ada@example.com", "-name", "Ada", "-password", "longpassword"}, strings.NewReader(""), ioDiscard(), &stderr, root, done, env)
	if code != 0 {
		t.Fatalf("exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "owner already exists, continuing") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestSetupShortPasswordAndMissingFlags(t *testing.T) {
	root := fakeRepo(t)
	var stderr strings.Builder
	code := setup([]string{"-org", "Acme", "-email", "ada@example.com", "-name", "Ada", "-password", "short"}, strings.NewReader(""), ioDiscard(), &stderr, root, nil, &setupEnv{})
	if code != 2 || !strings.Contains(stderr.String(), "password is required") {
		t.Fatalf("short %d %q", code, stderr.String())
	}
	stderr.Reset()
	code = setup([]string{"-org", "Acme"}, strings.NewReader(""), ioDiscard(), &stderr, root, nil, &setupEnv{})
	if code != 2 || !strings.Contains(stderr.String(), "-password") {
		t.Fatalf("missing %d %q", code, stderr.String())
	}
}

func TestSetupPrompts(t *testing.T) {
	root := fakeRepo(t)
	var calls []string
	env := recordingEnv(t, &calls)
	done := make(chan struct{})
	close(done)
	in := strings.NewReader("\nada@example.com\nAda\nlongpassword\n\n\n\n")
	var stdout, stderr strings.Builder
	code := setup(nil, in, &stdout, &stderr, root, done, env)
	if code != 0 {
		t.Fatalf("exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Organization [Acme]:") || !strings.Contains(stdout.String(), "Sign in as ada@example.com") {
		t.Fatalf("stdout %q", stdout.String())
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "seed -org Acme -email ada@example.com -name Ada -password longpassword") {
		t.Fatalf("calls %v", calls)
	}
}

func TestSetupURLSkipsEmbeddedDatabase(t *testing.T) {
	root := fakeRepo(t)
	started := false
	env := &setupEnv{
		startDB: func(string) (string, func(), error) {
			started = true
			return "", nil, errString("should not start")
		},
		run:   func(string, []string, string, ...string) error { return nil },
		start: func(string, []string, string, ...string) (func(), error) { return func() {}, nil },
		wait:  func(string) error { return nil },
	}
	done := make(chan struct{})
	close(done)
	code := setup([]string{"-org", "Acme", "-email", "ada@example.com", "-name", "Ada", "-password", "longpassword", "-database", "postgres://openforce:openforce@127.0.0.1:5432/openforce?sslmode=disable"}, strings.NewReader(""), ioDiscard(), ioDiscard(), root, done, env)
	if code != 0 || started {
		t.Fatalf("exit %d started %v", code, started)
	}
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "services", "api", "go.mod"),
		filepath.Join(root, "apps", "web", "package.json"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("module test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func recordingEnv(t *testing.T, calls *[]string) *setupEnv {
	t.Helper()
	return &setupEnv{
		startDB: func(string) (string, func(), error) {
			*calls = append(*calls, "db")
			return "postgres://openforce:openforce@127.0.0.1:5432/openforce?sslmode=disable", func() {}, nil
		},
		run: func(dir string, extra []string, name string, args ...string) error {
			*calls = append(*calls, name+" "+strings.Join(args, " "))
			return nil
		},
		start: func(dir string, extra []string, name string, args ...string) (func(), error) {
			*calls = append(*calls, name+" "+strings.Join(args, " "))
			return func() {}, nil
		},
		wait: func(string) error { return nil },
	}
}

func ioDiscard() *strings.Builder { return &strings.Builder{} }

type errString string

func (e errString) Error() string { return string(e) }
