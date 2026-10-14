package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func runSeed(args []string) int {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	org := fs.String("org", "", "organization name")
	email := fs.String("email", "", "owner email")
	name := fs.String("name", "", "owner name")
	password := fs.String("password", "", "owner password")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *org == "" || *email == "" || *name == "" {
		fmt.Fprintln(os.Stderr, "seed requires -org, -email, and -name")
		return 2
	}

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
	orgID, token, err := s.bootstrap(ctx, *org, *name, *email, "owner", *password)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("organization: %s\ntoken: %s\n", orgID, token)
	return 0
}
