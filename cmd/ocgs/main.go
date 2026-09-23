package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"opencode-go-switcher/internal/cli"
	"opencode-go-switcher/internal/store"
	"opencode-go-switcher/internal/verify"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := store.ResolveDataDir(os.Getenv, home)
	s := store.New(dir)
	v := &verify.Client{
		BaseURL:    verify.DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	cmd := cli.New(s, v, os.Stdout, os.Stderr)
	cmd.SetArgs(cli.NormalizeArgs(os.Args[1:]))
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
