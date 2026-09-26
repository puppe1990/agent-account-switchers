// Package codexlogin runs the Codex CLI sign in flow for the web UI: it starts
// `codex login` in the background, keeps the local callback server alive while
// the user authenticates and opens the authentication URL in a private browser
// window, so another account can sign in without touching the browser session
// that is already logged in.
package codexlogin

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"agent-account-switchers/internal/codexstore"
)

const (
	StateIdle    = "idle"
	StateRunning = "running"
	StateDone    = "done"
	StateError   = "error"
)

const startupWait = 20 * time.Second

// Status is the public view of the sign in flow.
type Status struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

type Options struct {
	Store   *codexstore.Store
	Command []string
	Open    func(string) error
	Timeout time.Duration
}

type Manager struct {
	store   *codexstore.Store
	command []string
	open    func(string) error
	timeout time.Duration

	shimOnce sync.Once
	shimEnv  []string

	mu     sync.Mutex
	gen    int
	proc   *exec.Cmd
	status Status
}

func New(opts Options) *Manager {
	m := &Manager{
		store:   opts.Store,
		command: opts.Command,
		open:    opts.Open,
		timeout: opts.Timeout,
		status:  Status{State: StateIdle},
	}
	if len(m.command) == 0 {
		m.command = []string{"codex", "login"}
	}
	if m.open == nil {
		m.open = OpenPrivate
	}
	if m.timeout <= 0 {
		m.timeout = 10 * time.Minute
	}
	return m
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Start begins a new sign in, cancelling one that is still running, and returns
// once the private window is open or the process has already failed.
func (m *Manager) Start() Status {
	m.mu.Lock()
	previous := m.proc
	m.gen++
	gen := m.gen
	m.proc = nil
	m.status = Status{State: StateRunning, Message: "Iniciando o login do Codex…"}
	m.mu.Unlock()
	if previous != nil && previous.Process != nil {
		_ = previous.Process.Kill()
	}

	_ = m.store.Sync()

	cmd := exec.Command(m.command[0], m.command[1:]...)
	cmd.Env = append(os.Environ(), m.shim()...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return m.update(gen, Status{State: StateError, Message: fmt.Sprintf("não consegui ler a saída do login: %v", err)})
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return m.update(gen, Status{State: StateError, Message: fmt.Sprintf("não consegui ler a saída do login: %v", err)})
	}
	if err := cmd.Start(); err != nil {
		return m.update(gen, Status{
			State:   StateError,
			Message: fmt.Sprintf("não consegui rodar %q: %v", strings.Join(m.command, " "), err),
		})
	}
	m.mu.Lock()
	if m.gen == gen {
		m.proc = cmd
	}
	m.mu.Unlock()

	opened := make(chan struct{})
	go m.run(gen, cmd, stdout, stderr, opened)

	select {
	case <-opened:
	case <-time.After(startupWait):
	}
	return m.Status()
}

func (m *Manager) run(gen int, cmd *exec.Cmd, stdout, stderr io.ReadCloser, opened chan struct{}) {
	closeOpened := sync.OnceFunc(func() { close(opened) })
	defer closeOpened()

	go func() { _, _ = io.Copy(io.Discard, stdout) }()

	timer := time.AfterFunc(m.timeout, func() { m.timeoutRun(gen) })
	defer timer.Stop()

	var url string
	var tail []string
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		tail = append(tail, line)
		if len(tail) > 3 {
			tail = tail[1:]
		}
		if url != "" || !strings.HasPrefix(line, "https://") {
			continue
		}
		url = line
		status := Status{State: StateRunning, Message: "Janela anônima aberta. Conclua o login do Codex."}
		if err := m.open(url); err != nil {
			status = Status{
				State:   StateRunning,
				Message: fmt.Sprintf("Não consegui abrir a janela anônima (%v). Abra o link para concluir o login.", err),
				URL:     url,
			}
		}
		m.update(gen, status)
		closeOpened()
	}

	m.finish(gen, cmd.Wait(), tail)
}

func (m *Manager) timeoutRun(gen int) {
	m.mu.Lock()
	if m.gen != gen {
		m.mu.Unlock()
		return
	}
	m.gen++
	m.status = Status{State: StateError, Message: "Tempo esgotado esperando o login do Codex."}
	proc := m.proc
	m.proc = nil
	m.mu.Unlock()
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Kill()
	}
}

func (m *Manager) finish(gen int, waitErr error, tail []string) {
	m.mu.Lock()
	stale := m.gen != gen
	if !stale {
		m.proc = nil
	}
	m.mu.Unlock()
	if stale {
		return
	}

	if waitErr != nil {
		message := "o login do Codex falhou"
		if line := errorLine(tail); line != "" {
			message = line
		}
		m.update(gen, Status{State: StateError, Message: message})
		return
	}

	name, err := m.store.ImportCurrent()
	if err != nil {
		m.update(gen, Status{
			State:   StateDone,
			Message: fmt.Sprintf("Login concluído, mas não consegui registrar a conta: %v", err),
		})
		return
	}
	m.update(gen, Status{State: StateDone, Message: fmt.Sprintf("Conta %q adicionada.", name)})
}

func (m *Manager) update(gen int, status Status) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gen == gen {
		m.status = status
	}
	return m.status
}

func errorLine(tail []string) string {
	for i := len(tail) - 1; i >= 0; i-- {
		if strings.Contains(tail[i], "Error") || strings.Contains(tail[i], "error") {
			return tail[i]
		}
	}
	if len(tail) > 0 {
		return tail[len(tail)-1]
	}
	return ""
}
