package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"opencode-go-switcher/internal/cccli"
	"opencode-go-switcher/internal/ccstore"
	"opencode-go-switcher/internal/ccverify"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := ccstore.ResolveDataDir(os.Getenv, home)
	s := ccstore.New(dir)
	v := &ccverify.Client{
		BaseURL:    ccverify.DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	cmd := cccli.New(s, v, os.Stdout, os.Stderr)
	cmd.SetArgs(cccli.NormalizeArgs(os.Args[1:]))
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
