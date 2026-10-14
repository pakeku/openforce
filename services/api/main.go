package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			os.Exit(runMigrate())
		case "seed":
			os.Exit(runSeed(os.Args[2:]))
		case "work":
			os.Exit(runWork(os.Args[2:]))
		default:
			fmt.Fprintln(os.Stderr, "usage: api [migrate|seed|work]")
			os.Exit(2)
		}
	}
	os.Exit(runServer())
}

func runMigrate() int {
	ctx := context.Background()
	dir, err := findMigrationsFrom(mustWD())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	conn, err := pgx.Connect(ctx, databaseURL())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer conn.Close(ctx)

	applied, err := applyMigrations(ctx, conn, dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, name := range applied {
		fmt.Println("applied", name)
	}
	if len(applied) == 0 {
		fmt.Println("migrations up to date")
	}
	return 0
}

func openPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	// Text parameters let Postgres coerce values onto uuid and jsonb columns.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["client_encoding"] = "UTF8"
	return pgxpool.NewWithConfig(ctx, cfg)
}

func runWork(args []string) int {
	once := false
	for _, arg := range args {
		if arg != "-once" {
			fmt.Fprintln(os.Stderr, "usage: api work [-once]")
			return 2
		}
		once = true
	}
	if once {
		return workSession(context.Background(), true)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return workSession(ctx, false)
}

func workSession(stop context.Context, once bool) int {
	ctx := context.Background()
	pool, err := openPool(ctx, databaseURL())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	client, err := openRedis(os.Getenv("REDIS_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if client != nil {
		defer client.Close()
	}
	s := &server{pool: pool, redis: client}
	if once {
		n, err := s.drainJobs(ctx, -1)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("delivered %d\n", n)
		return 0
	}
	return keepWorking(stop, s)
}

// drainJobs is one pass: schedules, then publish, then the stream, then Postgres.
// block is the stream read timeout. Negative skips the wait. It must not be 0.
func (s *server) drainJobs(ctx context.Context, block time.Duration) (int, error) {
	if _, err := s.enqueueDueSchedules(ctx); err != nil {
		return 0, err
	}
	if err := s.publishDueJobs(ctx); err != nil {
		return 0, err
	}
	queued, err := s.consumeQueuedJobs(ctx, block)
	if err != nil {
		return 0, err
	}
	polled, err := s.deliverJobs(ctx)
	if err != nil {
		return 0, err
	}
	return queued + polled, nil
}

func keepWorking(stop context.Context, s *server) int {
	// Redis waits inside the stream read. Without it, only an idle pass waits.
	block := time.Duration(-1)
	if s.redis != nil {
		block = time.Second
	}
	for {
		if stop.Err() != nil {
			return 0
		}
		n, err := s.drainJobs(context.Background(), block)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if n > 0 {
			fmt.Printf("delivered %d\n", n)
		}
		if stop.Err() != nil {
			return 0
		}
		if s.redis == nil && n == 0 {
			timer := time.NewTimer(time.Second)
			select {
			case <-stop.Done():
				timer.Stop()
				return 0
			case <-timer.C:
			}
		}
	}
}

func runServer() int {
	ctx := context.Background()
	pool, err := openPool(ctx, databaseURL())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	client, err := openRedis(os.Getenv("REDIS_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if client != nil {
		defer client.Close()
	}
	sharedRedis = client

	addr := listenAddr()
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, newMux(pool)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func databaseURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://openforce:openforce@localhost:5432/openforce?sslmode=disable"
}

func listenAddr() string {
	if addr := os.Getenv("ADDR"); addr != "" {
		return addr
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}

func mustWD() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}
