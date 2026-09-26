package codexlogin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"agent-account-switchers/internal/codexstore"
)

const (
	meKey   = "user-1::acc-1"
	newKey  = "user-3::acc-3"
	authURL = "https://auth.openai.com/oauth/authorize?x=1"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func accountFile(home, key string) string {
	name := base64.RawURLEncoding.EncodeToString([]byte(key)) + ".auth.json"
	return filepath.Join(home, "accounts", name)
}

func registryJSON(active string) string {
	return `{"schema_version":3,"active_account_key":"` + active + `","accounts":[` +
		`{"account_key":"` + meKey + `","chatgpt_account_id":"acc-1","chatgpt_user_id":"user-1",` +
		`"email":"me@example.com","alias":"","account_name":null,"plan":"free",` +
		`"auth_mode":"chatgpt","created_at":1774667021,"last_used_at":1779226449}]}`
}

func authJSON(accountID, marker string) string {
	return `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"` + marker +
		`","account_id":"` + accountID + `","id_token":"","refresh_token":"` + marker +
		`"},"last_refresh":"2026-09-24T18:53:00Z"}`
}

func jwt(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func chatGPTAuth(t *testing.T, accountID, userID, email, marker string) string {
	t.Helper()
	token := jwt(t, map[string]any{
		"email": email,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
			"chatgpt_user_id":    userID,
			"chatgpt_plan_type":  "plus",
		},
	})
	return `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"` + marker +
		`","account_id":"` + accountID + `","id_token":"` + token + `","refresh_token":"` + marker +
		`"},"last_refresh":"2026-09-24T18:53:00Z"}`
}

func loginScript(home, authBody string) string {
	body := `printf '%s\n' 'Starting local login server on http://localhost:1455.' ` +
		`'If your browser did not open, navigate to this URL to authenticate:' '' '` + authURL + `' >&2` + "\n"
	if authBody != "" {
		body += `printf '%s' '` + authBody + `' > '` + home + `/auth.json'` + "\n"
	}
	return body + "exit 0\n"
}

func newManager(t *testing.T, command []string, open func(string) error, timeout time.Duration) *Manager {
	t.Helper()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "registry.json"), registryJSON(meKey))
	return newManagerIn(t, home, command, open, timeout)
}

func newManagerIn(t *testing.T, home string, command []string, open func(string) error, timeout time.Duration) *Manager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("testes usam /bin/sh")
	}
	return New(Options{
		Store:   codexstore.New(home),
		Command: command,
		Open:    open,
		Timeout: timeout,
	})
}

func waitState(t *testing.T, m *Manager, want string) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if status := m.Status(); status.State == want {
			return status
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("estado = %q, quero %q", m.Status().State, want)
	return Status{}
}

func TestStartOpensPrivateWindowAndImportsAccount(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "registry.json"), registryJSON(meKey))
	body := chatGPTAuth(t, "acc-3", "user-3", "pessoal@example.com", "NEW")
	var opened []string
	open := func(url string) error {
		opened = append(opened, url)
		return nil
	}
	m := newManagerIn(t, home, []string{"/bin/sh", "-c", loginScript(home, body)}, open, 0)

	status := m.Start()
	if status.State != StateRunning {
		t.Fatalf("estado = %+v", status)
	}
	if len(opened) != 1 || opened[0] != authURL {
		t.Fatalf("janelas abertas = %v, quero [%s]", opened, authURL)
	}

	done := waitState(t, m, StateDone)
	if !strings.Contains(done.Message, "pessoal@example.com") {
		t.Fatalf("mensagem = %q", done.Message)
	}
	if got := readFile(t, accountFile(home, newKey)); got != body {
		t.Fatalf("credencial importada = %s", got)
	}
	entries, err := codexstore.New(home).List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 || !entries[1].Active || entries[1].Name != "pessoal@example.com" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestStartSyncsCurrentAccountBeforeLogin(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "registry.json"), registryJSON(meKey))
	writeFile(t, accountFile(home, meKey), authJSON("acc-1", "ME"))
	live := authJSON("acc-1", "ME-REFRESHED")
	writeFile(t, filepath.Join(home, "auth.json"), live)

	m := newManagerIn(t, home, []string{"/bin/sh", "-c", "echo 'Error logging in: sem rede' >&2\nexit 1\n"}, nil, 0)
	status := m.Start()
	if status.State != StateError {
		t.Fatalf("estado = %+v", status)
	}
	if !strings.Contains(status.Message, "Error logging in") {
		t.Fatalf("mensagem = %q", status.Message)
	}
	if got := readFile(t, accountFile(home, meKey)); got != live {
		t.Fatalf("conta atual não foi sincronizada antes do login: %s", got)
	}
}

func TestStartWithoutURLFails(t *testing.T) {
	m := newManager(t, []string{"/bin/sh", "-c", "echo 'Error logging in: boom' >&2\nexit 1\n"}, nil, 0)

	status := m.Start()
	if status.State != StateError {
		t.Fatalf("estado = %+v", status)
	}
	if !strings.Contains(status.Message, "Error logging in: boom") {
		t.Fatalf("mensagem = %q", status.Message)
	}
}

func TestStartWhenOpenFailsExposesURL(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "registry.json"), registryJSON(meKey))
	command := []string{"/bin/sh", "-c", "echo '" + authURL + "' >&2\nsleep 5\n"}
	m := newManagerIn(t, home, command, func(string) error {
		return fmt.Errorf("sem navegador")
	}, 0)

	status := m.Start()
	if status.State != StateRunning {
		t.Fatalf("estado = %+v", status)
	}
	if status.URL != authURL {
		t.Fatalf("url = %q, quero %q", status.URL, authURL)
	}
	if !strings.Contains(status.Message, "sem navegador") {
		t.Fatalf("mensagem = %q", status.Message)
	}
}

func TestStartTimesOut(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "registry.json"), registryJSON(meKey))
	command := []string{"/bin/sh", "-c", "echo '" + authURL + "' >&2\nsleep 30\n"}
	m := newManagerIn(t, home, command, func(string) error { return nil }, 200*time.Millisecond)

	if status := m.Start(); status.State != StateRunning {
		t.Fatalf("estado = %+v", status)
	}
	status := waitState(t, m, StateError)
	if !strings.Contains(status.Message, "Tempo esgotado") {
		t.Fatalf("mensagem = %q", status.Message)
	}
}

func TestStartReplacesRunningLogin(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "registry.json"), registryJSON(meKey))
	body := chatGPTAuth(t, "acc-3", "user-3", "pessoal@example.com", "NEW")
	slow := []string{"/bin/sh", "-c", "echo '" + authURL + "' >&2\nsleep 30\n"}
	m := newManagerIn(t, home, slow, func(string) error { return nil }, 0)

	if status := m.Start(); status.State != StateRunning {
		t.Fatalf("primeiro start = %+v", status)
	}
	m.command = []string{"/bin/sh", "-c", loginScript(home, body)}

	status := m.Start()
	if status.State != StateRunning {
		t.Fatalf("segundo start = %+v", status)
	}
	waitState(t, m, StateDone)
	time.Sleep(100 * time.Millisecond)
	if status := m.Status(); status.State != StateDone {
		t.Fatalf("processo antigo mexeu no estado: %+v", status)
	}
}

func TestStatusStartsIdle(t *testing.T) {
	m := newManager(t, nil, nil, 0)
	if status := m.Status(); status.State != StateIdle {
		t.Fatalf("estado inicial = %+v", status)
	}
}

func TestCommandNotFound(t *testing.T) {
	m := newManager(t, []string{"codex-que-nao-existe"}, nil, 0)
	status := m.Start()
	if status.State != StateError {
		t.Fatalf("estado = %+v", status)
	}
	if !strings.Contains(status.Message, "não consegui rodar") {
		t.Fatalf("mensagem = %q", status.Message)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
