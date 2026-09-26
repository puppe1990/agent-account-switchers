package codexstore

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	meKey   = "user-1::acc-1"
	workKey = "user-2::acc-2"
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

func writeAccountAuth(t *testing.T, home, key, body string) {
	t.Helper()
	writeFile(t, accountFile(home, key), body)
}

func writeLiveAuth(t *testing.T, home, body string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "auth.json"), body)
}

func writeRegistry(t *testing.T, home, body string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "accounts", "registry.json"), body)
}

func authJSON(accountID, marker string) string {
	return `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"` +
		marker + `","account_id":"` + accountID + `","id_token":"` + marker +
		`","refresh_token":"` + marker + `"},"last_refresh":"2026-09-24T18:53:00Z"}`
}

func registryJSON(activeKey string) string {
	return `{
  "schema_version": 3,
  "active_account_key": "` + activeKey + `",
  "active_account_activated_at_ms": 1779226449942,
  "auto_switch": {"enabled": true, "threshold_5h_percent": 10},
  "api": {"usage": true, "account": true},
  "accounts": [
    {
      "account_key": "` + meKey + `",
      "chatgpt_account_id": "acc-1",
      "chatgpt_user_id": "user-1",
      "email": "me@example.com",
      "alias": "",
      "account_name": null,
      "plan": "free",
      "auth_mode": "chatgpt",
      "created_at": 1774667021,
      "last_used_at": 1779226449
    },
    {
      "account_key": "` + workKey + `",
      "chatgpt_account_id": "acc-2",
      "chatgpt_user_id": "user-2",
      "email": "work@example.com",
      "alias": "work",
      "account_name": "Time",
      "plan": "plus",
      "auth_mode": "chatgpt",
      "created_at": 1774667021,
      "last_used_at": 1779226449,
      "last_usage": {"primary": {"used_percent": 100, "resets_at": 1790650142}}
    }
  ]
}`
}

func readRegistry(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "accounts", "registry.json"))
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	var reg map[string]any
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("registry inválido: %v", err)
	}
	return reg
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func registryAccount(t *testing.T, reg map[string]any, key string) map[string]any {
	t.Helper()
	raw, _ := reg["accounts"].([]any)
	for _, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if fields["account_key"] == key {
			return fields
		}
	}
	t.Fatalf("conta %s não está no registry", key)
	return nil
}

func TestListEmptyWhenNoRegistry(t *testing.T) {
	entries, err := New(t.TempDir()).List()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %v, quero vazio", entries)
	}
}

func TestListMarksActiveFromRegistry(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))

	entries, err := New(home).List()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %v, quero 2", entries)
	}
	if entries[0].Name != "me@example.com" || !entries[0].Active {
		t.Fatalf("esperava me@example.com ativa, veio %+v", entries[0])
	}
	if entries[1].Name != "work" || entries[1].Active {
		t.Fatalf("esperava work inativa, veio %+v", entries[1])
	}
}

func TestListNamePrefersAliasThenAccountNameThenEmailThenKey(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, `{"schema_version":3,"active_account_key":null,"accounts":[
  {"account_key":"k-alias","alias":"trabalho","account_name":"Time","email":"a@example.com"},
  {"account_key":"k-name","account_name":"Time","email":"b@example.com"},
  {"account_key":"k-email","email":"pessoal@example.com"},
  {"account_key":"k-key"}
]}`)

	entries, err := New(home).List()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	want := []string{"Time", "k-key", "pessoal@example.com", "trabalho"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, quero %v", names, want)
	}
}

func TestSwitchWritesAuthAndUpdatesRegistry(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))
	writeAccountAuth(t, home, workKey, authJSON("acc-2", "WORK"))
	live := authJSON("acc-1", "ME-REFRESHED")
	writeLiveAuth(t, home, live)

	s := New(home)
	s.Now = func() time.Time { return time.Unix(1780000000, 0) }
	if err := s.Switch("work"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	authPath := filepath.Join(home, "auth.json")
	if got := readFile(t, authPath); got != authJSON("acc-2", "WORK") {
		t.Fatalf("auth.json = %s", got)
	}
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("stat auth: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth.json com modo %v, quero 0600", info.Mode().Perm())
	}

	reg := readRegistry(t, home)
	if reg["active_account_key"] != workKey {
		t.Fatalf("active_account_key = %v, quero %s", reg["active_account_key"], workKey)
	}
	if reg["active_account_activated_at_ms"] != float64(1780000000000) {
		t.Fatalf("activated_at_ms = %v", reg["active_account_activated_at_ms"])
	}
	if tokens := readFile(t, accountFile(home, meKey)); tokens != live {
		t.Fatalf("conta ativa anterior não foi sincronizada: %s", tokens)
	}
	work := registryAccount(t, reg, workKey)
	if work["last_used_at"] != float64(1780000000) {
		t.Fatalf("last_used_at = %v, quero 1780000000", work["last_used_at"])
	}
	me := registryAccount(t, reg, meKey)
	if me["last_used_at"] != float64(1779226449) {
		t.Fatalf("last_used_at da conta anterior mudou: %v", me["last_used_at"])
	}
	if _, ok := work["last_usage"]; !ok {
		t.Fatal("last_usage da conta alvo foi perdido")
	}
	autoSwitch, _ := reg["auto_switch"].(map[string]any)
	if autoSwitch["enabled"] != true {
		t.Fatalf("auto_switch foi perdido: %v", reg["auto_switch"])
	}
	api, _ := reg["api"].(map[string]any)
	if api["usage"] != true {
		t.Fatalf("api foi perdido: %v", reg["api"])
	}
}

func TestSwitchByAliasEmailAndKey(t *testing.T) {
	for _, ref := range []string{"work", "WORK", "work@example.com", workKey} {
		t.Run(ref, func(t *testing.T) {
			home := t.TempDir()
			writeRegistry(t, home, registryJSON(meKey))
			writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))
			writeAccountAuth(t, home, workKey, authJSON("acc-2", "WORK"))

			if err := New(home).Switch(ref); err != nil {
				t.Fatalf("switch %q: %v", ref, err)
			}
			if got := readFile(t, filepath.Join(home, "auth.json")); got != authJSON("acc-2", "WORK") {
				t.Fatalf("auth.json = %s", got)
			}
		})
	}
}

func TestSwitchAlreadyActiveKeepsLiveTokens(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))
	live := authJSON("acc-1", "ME-REFRESHED")
	writeLiveAuth(t, home, live)

	if err := New(home).Switch("me@example.com"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if got := readFile(t, filepath.Join(home, "auth.json")); got != live {
		t.Fatalf("auth.json perdeu os tokens atualizados: %s", got)
	}
}

func TestSwitchKeepsUnmatchedLiveAuthOutOfStoredAccounts(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	savedMe := authJSON("acc-1", "ME")
	savedWork := authJSON("acc-2", "WORK")
	writeAccountAuth(t, home, meKey, savedMe)
	writeAccountAuth(t, home, workKey, savedWork)
	writeLiveAuth(t, home, authJSON("acc-9", "OUTRA"))

	if err := New(home).Switch("work"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if got := readFile(t, accountFile(home, meKey)); got != savedMe {
		t.Fatalf("conta me foi sobrescrita: %s", got)
	}
	if got := readFile(t, accountFile(home, workKey)); got != savedWork {
		t.Fatalf("conta work foi sobrescrita: %s", got)
	}
}

func TestSwitchFindsFileByCodexAuthEncoding(t *testing.T) {
	home := t.TempDir()
	const key = "user-kJcQbqUPq7yMES0ALoCeVu1G::ccda98f9-75f5-47a4-b092-e84434db293f"
	const name = "dXNlci1rSmNRYnFVUHE3eU1FUzBBTG9DZVZ1MUc6OmNjZGE5OGY5LTc1ZjUtNDdhNC1iMDkyLWU4NDQzNGRiMjkzZg.auth.json"
	writeRegistry(t, home, `{"schema_version":3,"active_account_key":null,"accounts":[{"account_key":"`+key+`","email":"puppeicaropuppe@gmail.com"}]}`)
	writeFile(t, filepath.Join(home, "accounts", name), authJSON("ccda98f9", "PESSOAL"))

	if err := New(home).Switch(key); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if got := readFile(t, filepath.Join(home, "auth.json")); got != authJSON("ccda98f9", "PESSOAL") {
		t.Fatalf("auth.json = %s", got)
	}
}

func TestSwitchUnknown(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))

	err := New(home).Switch("nao-existe")
	if err == nil {
		t.Fatal("esperava erro para conta inexistente")
	}
	if !strings.Contains(err.Error(), "não existe") {
		t.Fatalf("erro = %v", err)
	}
}

func TestSwitchWithoutAccounts(t *testing.T) {
	if err := New(t.TempDir()).Switch("qualquer"); err == nil {
		t.Fatal("esperava erro sem contas salvas")
	}
}

func TestSwitchEmptyName(t *testing.T) {
	if err := New(t.TempDir()).Switch("  "); err == nil {
		t.Fatal("esperava erro para nome vazio")
	}
}

func TestSwitchMissingCredentialFile(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))

	err := New(home).Switch("work")
	if err == nil {
		t.Fatal("esperava erro para credencial ausente")
	}
	if !strings.Contains(err.Error(), "não achei a credencial") {
		t.Fatalf("erro = %v", err)
	}
}

func TestSwitchCorruptCredential(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))
	writeAccountAuth(t, home, workKey, "{")

	err := New(home).Switch("work")
	if err == nil {
		t.Fatal("esperava erro para credencial corrompida")
	}
	if !strings.Contains(err.Error(), "não é um JSON válido") {
		t.Fatalf("erro = %v", err)
	}
}

func TestSwitchCorruptRegistry(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, "{")

	if err := New(home).Switch("work"); err == nil {
		t.Fatal("esperava erro para registry corrompido")
	}
}

func TestResolveDataDir(t *testing.T) {
	if got := ResolveDataDir(func(string) string { return "" }, "/home/me"); got != "/home/me/.codex" {
		t.Fatalf("dir = %q, quero o default ~/.codex", got)
	}
	if got := ResolveDataDir(func(key string) string {
		if key == "CODEX_HOME" {
			return "/custom/codex"
		}
		return ""
	}, "/home/me"); got != "/custom/codex" {
		t.Fatalf("dir = %q, quero CODEX_HOME", got)
	}
}

func jwt(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func claimsFor(userID, accountID, email string) map[string]any {
	return map[string]any{
		"email": email,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
			"chatgpt_user_id":    userID,
			"chatgpt_plan_type":  "plus",
		},
	}
}

func chatGPTAuth(t *testing.T, accountID, marker string, claims map[string]any) string {
	t.Helper()
	return `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"` + marker +
		`","account_id":"` + accountID + `","id_token":"` + jwt(t, claims) + `","refresh_token":"` + marker +
		`"},"last_refresh":"2026-09-24T18:53:00Z"}`
}

func TestSyncUpdatesSavedAccount(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))
	live := authJSON("acc-1", "ME-REFRESHED")
	writeLiveAuth(t, home, live)

	if err := New(home).Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readFile(t, accountFile(home, meKey)); got != live {
		t.Fatalf("conta me não foi sincronizada: %s", got)
	}
}

func TestImportCurrentAddsAccount(t *testing.T) {
	const newKey = "user-3::acc-3"
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))
	body := chatGPTAuth(t, "acc-3", "NEW", claimsFor("user-3", "acc-3", "pessoal@example.com"))
	writeLiveAuth(t, home, body)

	s := New(home)
	s.Now = func() time.Time { return time.Unix(1780000000, 0) }
	name, err := s.ImportCurrent()
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if name != "pessoal@example.com" {
		t.Fatalf("name = %q, quero pessoal@example.com", name)
	}
	if got := readFile(t, accountFile(home, newKey)); got != body {
		t.Fatalf("credencial importada = %s", got)
	}

	reg := readRegistry(t, home)
	if reg["active_account_key"] != newKey {
		t.Fatalf("active_account_key = %v, quero %s", reg["active_account_key"], newKey)
	}
	added := registryAccount(t, reg, newKey)
	if added["email"] != "pessoal@example.com" || added["plan"] != "plus" || added["chatgpt_user_id"] != "user-3" {
		t.Fatalf("registro importado = %v", added)
	}
	if added["last_used_at"] != float64(1780000000) {
		t.Fatalf("last_used_at = %v", added["last_used_at"])
	}

	entries, err := New(home).List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 || !entries[1].Active || entries[1].Name != "pessoal@example.com" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestImportCurrentUpdatesExistingAccount(t *testing.T) {
	home := t.TempDir()
	writeRegistry(t, home, registryJSON(meKey))
	writeAccountAuth(t, home, meKey, authJSON("acc-1", "ME"))
	writeLiveAuth(t, home, chatGPTAuth(t, "acc-1", "ME2", claimsFor("user-1", "acc-1", "novo@example.com")))

	s := New(home)
	s.Now = func() time.Time { return time.Unix(1780000000, 0) }
	name, err := s.ImportCurrent()
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if name != "novo@example.com" {
		t.Fatalf("name = %q", name)
	}
	reg := readRegistry(t, home)
	if raw, _ := reg["accounts"].([]any); len(raw) != 2 {
		t.Fatalf("registry duplicou contas: %v", raw)
	}
	me := registryAccount(t, reg, meKey)
	if me["email"] != "novo@example.com" {
		t.Fatalf("email = %v", me["email"])
	}
	if me["created_at"] != float64(1774667021) {
		t.Fatalf("created_at mudou: %v", me["created_at"])
	}
}

func TestImportCurrentCreatesRegistry(t *testing.T) {
	home := t.TempDir()
	writeLiveAuth(t, home, chatGPTAuth(t, "acc-2", "WORK", claimsFor("user-2", "acc-2", "work@example.com")))

	s := New(home)
	s.Now = func() time.Time { return time.Unix(1780000000, 0) }
	if _, err := s.ImportCurrent(); err != nil {
		t.Fatalf("import: %v", err)
	}
	reg := readRegistry(t, home)
	if reg["schema_version"] != float64(3) {
		t.Fatalf("schema_version = %v", reg["schema_version"])
	}
	if reg["active_account_key"] != workKey {
		t.Fatalf("active_account_key = %v", reg["active_account_key"])
	}
}

func TestImportCurrentWithoutChatGPTAuth(t *testing.T) {
	home := t.TempDir()
	writeLiveAuth(t, home, `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-abc","tokens":null}`)

	if _, err := New(home).ImportCurrent(); err == nil {
		t.Fatal("esperava erro para auth sem conta ChatGPT")
	}
}

func TestImportCurrentMissingAuth(t *testing.T) {
	if _, err := New(t.TempDir()).ImportCurrent(); err == nil {
		t.Fatal("esperava erro sem auth.json")
	}
}

func TestImportCurrentRejectsMismatchedIDs(t *testing.T) {
	home := t.TempDir()
	writeLiveAuth(t, home, chatGPTAuth(t, "acc-9", "X", claimsFor("user-2", "acc-2", "work@example.com")))

	err := func() error {
		_, err := New(home).ImportCurrent()
		return err
	}()
	if err == nil {
		t.Fatal("esperava erro para id_token e account_id diferentes")
	}
	if !strings.Contains(err.Error(), "não batem") {
		t.Fatalf("erro = %v", err)
	}
}
