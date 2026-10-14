package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: openforce pull|push|setup")
		return 2
	}
	cmd := args[0]
	switch cmd {
	case "setup":
		return setup(args[1:], os.Stdin, os.Stdout, stderr, "", nil, nil)
	case "pull", "push":
	default:
		fmt.Fprintf(stderr, "unknown command %s\n", cmd)
		return 2
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: openforce %s -url URL -token TOKEN -dir DIR\n", cmd)
	}
	url := fs.String("url", "", "API base URL")
	token := fs.String("token", "", "bearer token")
	dir := fs.String("dir", "", "project directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected %s\n", fs.Arg(0))
		return 2
	}
	missing := false
	if *url == "" {
		fmt.Fprintln(stderr, "missing -url")
		missing = true
	}
	if *token == "" {
		fmt.Fprintln(stderr, "missing -token")
		missing = true
	}
	if *dir == "" {
		fmt.Fprintln(stderr, "missing -dir")
		missing = true
	}
	if missing {
		return 2
	}
	var err error
	switch cmd {
	case "pull":
		err = pull(*url, *token, *dir)
	case "push":
		err = push(*url, *token, *dir)
	}
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	return 0
}
