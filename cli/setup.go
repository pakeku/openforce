package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"golang.org/x/term"
)

type devConfig struct {
	Org      string `json:"org"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Web      int    `json:"web"`
	Database string `json:"database"`
}

type setupEnv struct {
	startDB func(root string) (url string, stop func(), err error)
	run     func(dir string, extra []string, name string, args ...string) error
	start   func(dir string, extra []string, name string, args ...string) (stop func(), err error)
	wait    func(url string) error
}

func setup(args []string, in io.Reader, stdout, stderr io.Writer, start string, done <-chan struct{}, env *setupEnv) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: openforce setup [-org NAME -email EMAIL -name NAME -password PASSWORD] [-port N] [-web N] [-database embedded|URL] [-reuse]")
	}
	org := fs.String("org", "", "organization name")
	email := fs.String("email", "", "owner email")
	name := fs.String("name", "", "owner name")
	password := fs.String("password", "", "owner password")
	port := fs.Int("port", 0, "API port")
	web := fs.Int("web", 0, "web port")
	database := fs.String("database", "", "embedded or a postgres URL")
	reuse := fs.Bool("reuse", false, "start from .openforce/dev.json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected %s\n", fs.Arg(0))
		return 2
	}

	root, err := findRoot(start)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	saved, savedOK := readConfig(root)
	cfg := devConfig{Org: "Acme", Port: 8080, Web: 5173, Database: "embedded"}
	askPassword := true
	flags := *org != "" || *email != "" || *name != "" || *password != "" || *reuse
	reader := bufio.NewReader(in)
	switch {
	case *reuse:
		if !savedOK {
			fmt.Fprintln(stderr, "no saved setup")
			return 2
		}
		cfg = saved
		askPassword = *password != ""
	case flags:
		if *org == "" || *email == "" || *name == "" || *password == "" {
			fmt.Fprintln(stderr, "setup needs -org, -email, -name, and -password")
			return 2
		}
		cfg.Org = *org
		cfg.Email = *email
		cfg.Name = *name
	default:
		useSaved := false
		if savedOK {
			answer, err := ask(reader, stdout, fmt.Sprintf("Use saved setup for %s <%s>? [Y/n]: ", saved.Name, saved.Email), "")
			if err != nil {
				fmt.Fprintln(stderr, err.Error())
				return 1
			}
			useSaved = answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
		}
		if useSaved {
			cfg = saved
			askPassword = false
		} else {
			filled, secret, err := promptConfig(in, reader, stdout, cfg)
			if err != nil {
				fmt.Fprintln(stderr, err.Error())
				return 1
			}
			cfg = filled
			*password = secret
		}
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *web != 0 {
		cfg.Web = *web
	}
	if *database != "" {
		cfg.Database = *database
	}
	if cfg.Port < 1 || cfg.Port > 65535 || cfg.Web < 1 || cfg.Web > 65535 {
		fmt.Fprintln(stderr, "invalid port")
		return 2
	}
	if askPassword && len(*password) < 8 {
		fmt.Fprintln(stderr, "password is required")
		return 2
	}
	if strings.TrimSpace(cfg.Org) == "" || strings.TrimSpace(cfg.Email) == "" || strings.TrimSpace(cfg.Name) == "" {
		fmt.Fprintln(stderr, "setup needs an organization, email, and name")
		return 2
	}
	if err := writeConfig(root, cfg); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	if env == nil {
		env = realEnv()
	}

	var stops []func()
	defer func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
	}()

	databaseURL := cfg.Database
	if databaseURL == "" || databaseURL == "embedded" {
		url, stop, err := env.startDB(root)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		databaseURL = url
		stops = append(stops, stop)
	}
	apiDir := filepath.Join(root, "services", "api")
	dbEnv := []string{"DATABASE_URL=" + databaseURL}
	if err := env.run(apiDir, dbEnv, "go", "run", ".", "migrate"); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	if askPassword {
		err := env.run(apiDir, dbEnv, "go", "run", ".", "seed", "-org", cfg.Org, "-email", cfg.Email, "-name", cfg.Name, "-password", *password)
		if err != nil && !seedExists(err) {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		if err != nil {
			fmt.Fprintln(stderr, "owner already exists, continuing")
		}
	}
	apiStop, err := env.start(apiDir, append(dbEnv, fmt.Sprintf("PORT=%d", cfg.Port)), "go", "run", ".")
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	stops = append(stops, apiStop)
	apiURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
	if err := env.wait(apiURL + "/healthz"); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	webDir := filepath.Join(root, "apps", "web")
	webStop, err := env.start(webDir, []string{"VITE_API_URL=" + apiURL}, "npm", "run", "dev", "--", "--host", "127.0.0.1", "--port", strconv.Itoa(cfg.Web), "--strictPort")
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	stops = append(stops, webStop)
	webURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Web)
	if err := env.wait(webURL); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	fmt.Fprintf(stdout, "API %s\nWeb %s\nSign in as %s\n", apiURL, webURL, cfg.Email)
	if done == nil {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		signal.Stop(sig)
	} else {
		<-done
	}
	return 0
}

func seedExists(err error) bool {
	text := err.Error()
	return strings.Contains(text, "duplicate key") || strings.Contains(text, "23505")
}

func promptConfig(in io.Reader, reader *bufio.Reader, out io.Writer, base devConfig) (devConfig, string, error) {
	org, err := ask(reader, out, "Organization ["+base.Org+"]: ", base.Org)
	if err != nil {
		return devConfig{}, "", err
	}
	email, err := ask(reader, out, "Email: ", "")
	if err != nil {
		return devConfig{}, "", err
	}
	owner, err := ask(reader, out, "Name: ", "")
	if err != nil {
		return devConfig{}, "", err
	}
	secret, err := askSecret(in, reader, out)
	if err != nil {
		return devConfig{}, "", err
	}
	portText, err := ask(reader, out, fmt.Sprintf("API port [%d]: ", base.Port), strconv.Itoa(base.Port))
	if err != nil {
		return devConfig{}, "", err
	}
	webText, err := ask(reader, out, fmt.Sprintf("Web port [%d]: ", base.Web), strconv.Itoa(base.Web))
	if err != nil {
		return devConfig{}, "", err
	}
	database, err := ask(reader, out, "Postgres URL (empty for a local database): ", "embedded")
	if err != nil {
		return devConfig{}, "", err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return devConfig{}, "", fmt.Errorf("invalid port")
	}
	web, err := strconv.Atoi(webText)
	if err != nil {
		return devConfig{}, "", fmt.Errorf("invalid port")
	}
	return devConfig{Org: org, Email: email, Name: owner, Port: port, Web: web, Database: database}, secret, nil
}

func ask(reader *bufio.Reader, out io.Writer, label, fallback string) (string, error) {
	fmt.Fprint(out, label)
	text, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback, nil
	}
	return text, nil
}

func askSecret(in io.Reader, reader *bufio.Reader, out io.Writer) (string, error) {
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(out, "Password: ")
		buf, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	}
	return ask(reader, out, "Password: ", "")
}

func findRoot(start string) (string, error) {
	dir := start
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	for {
		api := filepath.Join(dir, "services", "api", "go.mod")
		web := filepath.Join(dir, "apps", "web", "package.json")
		if fileExists(api) && fileExists(web) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("run setup from the openforce repository")
		}
		dir = parent
	}
}

func configPath(root string) string {
	return filepath.Join(root, ".openforce", "dev.json")
}

func readConfig(root string) (devConfig, bool) {
	raw, err := os.ReadFile(configPath(root))
	if err != nil {
		return devConfig{}, false
	}
	var cfg devConfig
	if json.Unmarshal(raw, &cfg) != nil || cfg.Email == "" {
		return devConfig{}, false
	}
	return cfg, true
}

func writeConfig(root string, cfg devConfig) error {
	if cfg.Database == "" {
		cfg.Database = "embedded"
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(root, ".openforce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(configPath(root), append(raw, '\n'), 0o600)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func realEnv() *setupEnv {
	return &setupEnv{startDB: startEmbedded, run: runCommand, start: startCommand, wait: waitReady}
}

func startEmbedded(root string) (string, func(), error) {
	port := 5432
	if !canListen(port) {
		spare, err := freePort()
		if err != nil {
			return "", nil, err
		}
		port = spare
	}
	dir := filepath.Join(root, ".openforce", "pg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	db := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(uint32(port)).
		Database("openforce").
		Username("openforce").
		Password("openforce").
		RuntimePath(dir).
		Locale("C").
		Encoding("UTF8").
		StartTimeout(5 * time.Minute).
		Logger(io.Discard))
	if err := db.Start(); err != nil {
		return "", nil, err
	}
	url := fmt.Sprintf("postgres://openforce:openforce@127.0.0.1:%d/openforce?sslmode=disable", port)
	return url, func() { _ = db.Stop() }, nil
}

func canListen(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

func runCommand(dir string, extra []string, name string, args ...string) error {
	cmd, err := script(dir, extra, nil, name, args...)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w\n%s", err, buf.String())
	}
	return nil
}

func startCommand(dir string, extra []string, name string, args ...string) (func(), error) {
	cmd, err := script(dir, extra, nil, name, args...)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() { stopProcess(cmd) }, nil
}

func script(dir string, extra []string, out io.Writer, name string, args ...string) (*exec.Cmd, error) {
	if _, err := exec.LookPath(name); err != nil {
		return nil, fmt.Errorf("%s is required", name)
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", append([]string{"/c", name}, args...)...)
	} else {
		cmd = exec.Command(name, args...)
	}
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extra...)
	if out != nil {
		cmd.Stdout = out
		cmd.Stderr = out
	}
	return cmd, nil
}

func stopProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		kill := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		kill.Stdout = io.Discard
		kill.Stderr = io.Discard
		_ = kill.Run()
		return
	}
	_ = cmd.Process.Kill()
}

func waitReady(url string) error {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", url)
}
