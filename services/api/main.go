package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

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
			os.Exit(runWork())
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

func runWork() int {
	ctx := context.Background()
	pool, err := openPool(ctx, databaseURL())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	n, err := (&server{pool: pool}).deliverJobs(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("delivered %d\n", n)
	return 0
}

func runServer() int {
	ctx := context.Background()
	pool, err := openPool(ctx, databaseURL())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()

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
