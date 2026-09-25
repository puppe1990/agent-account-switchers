package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"agent-account-switchers/internal/ccstore"
	"agent-account-switchers/internal/store"
	"agent-account-switchers/internal/webui"
)

func main() {
	port := flag.Int("port", envPort(8765), "porta do servidor local")
	noOpen := flag.Bool("no-open", false, "não abrir o navegador")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	goDir := store.ResolveDataDir(os.Getenv, home)
	ccDir := ccstore.ResolveDataDir(os.Getenv, home)

	server := webui.New(
		webui.NamedService{ID: "opencode-go", Label: "OpenCode Go", Service: goAdapter{store.New(goDir)}},
		webui.NamedService{ID: "commandcode", Label: "Command Code", Service: ccAdapter{ccstore.New(ccDir)}},
	)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "não consegui abrir %s: %v\n", addr, err)
		os.Exit(1)
	}
	url := fmt.Sprintf("http://%s/", listener.Addr().String())
	fmt.Printf("Switcher UI em %s (Ctrl+C para sair)\n", url)
	if !*noOpen {
		openBrowser(url)
	}

	srv := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envPort(fallback int) int {
	if raw := os.Getenv("SWITCHER_UI_PORT"); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			return port
		}
	}
	return fallback
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

type goAdapter struct{ store *store.Store }

func (a goAdapter) List() ([]webui.Account, error) {
	entries, err := a.store.List()
	if err != nil {
		return nil, err
	}
	accounts := make([]webui.Account, 0, len(entries))
	for _, e := range entries {
		accounts = append(accounts, webui.Account{Name: e.Name, Active: e.Active})
	}
	return accounts, nil
}

func (a goAdapter) Switch(name string) error { return a.store.Switch(name) }

type ccAdapter struct{ store *ccstore.Store }

func (a ccAdapter) List() ([]webui.Account, error) {
	entries, err := a.store.List()
	if err != nil {
		return nil, err
	}
	accounts := make([]webui.Account, 0, len(entries))
	for _, e := range entries {
		accounts = append(accounts, webui.Account{Name: e.Name, Active: e.Active})
	}
	return accounts, nil
}

func (a ccAdapter) Switch(name string) error { return a.store.Switch(name) }
