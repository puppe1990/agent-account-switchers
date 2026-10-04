package grokstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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

func writeProfile(t *testing.T, home, alias, email, auth string) {
	t.Helper()
	body := `{"alias":"` + alias + `","email":"` + email + `","auth":` + auth + `}`
	writeFile(t, filepath.Join(home, "accounts", alias+".json"), body)
}

func writeAuth(t *testing.T, home, email string) {
	t.Helper()
	body := `{"https://auth.x.ai::abc":{"email":"` + email + `"}}`
	writeFile(t, filepath.Join(home, "auth.json"), body)
}

func TestListEmptyWhenNoDir(t *testing.T) {
	entries, err := New(t.TempDir()).List()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %v, quero vazio", entries)
	}
}

func TestListMarksActiveByEmail(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"k":{"email":"work@example.com"}}`)
	writeProfile(t, home, "personal", "me@example.com", `{"k":{"email":"me@example.com"}}`)
	writeAuth(t, home, "me@example.com")

	entries, err := New(home).List()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %v, quero 2", entries)
	}
	if entries[0].Name != "personal" || !entries[0].Active {
		t.Fatalf("esperava personal ativa, veio %+v", entries[0])
	}
	if entries[1].Name != "work" || entries[1].Active {
		t.Fatalf("esperava work inativa, veio %+v", entries[1])
	}
}

func TestListNoAuthNoneActive(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"k":{"email":"work@example.com"}}`)

	entries, err := New(home).List()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(entries) != 1 || entries[0].Active {
		t.Fatalf("nenhuma conta deveria estar ativa: %+v", entries)
	}
}

func TestListNameFallsBackToEmail(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "me.json"),
		`{"alias":"","email":"me@example.com","auth":{"k":{"email":"me@example.com"}}}`)

	entries, err := New(home).List()
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if entries[0].Name != "me@example.com" {
		t.Fatalf("name = %q, quero o email", entries[0].Name)
	}
}

func TestSwitchWritesAuthAndFlipsActive(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"k":{"email":"work@example.com"}}`)
	writeProfile(t, home, "personal", "me@example.com", `{"k":{"email":"me@example.com"}}`)
	writeAuth(t, home, "me@example.com")

	if err := New(home).Switch("work"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		t.Fatalf("read auth: %v", err)
	}
	if string(data) != `{"k":{"email":"work@example.com"}}` {
		t.Fatalf("auth.json = %s", data)
	}
	entries, err := New(home).List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !entries[1].Active || entries[1].Name != "work" {
		t.Fatalf("work deveria estar ativa: %+v", entries)
	}
}

func TestSwitchByEmail(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "Work@Example.com", `{"k":{"email":"Work@Example.com"}}`)
	if err := New(home).Switch("work@example.com"); err != nil {
		t.Fatalf("switch por email: %v", err)
	}
}

func TestSwitchUnknown(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"token":"WORK"}`)
	if err := New(home).Switch("nao-existe"); err == nil {
		t.Fatal("esperava erro para perfil inexistente")
	}
}

func TestSwitchCorruptProfile(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "accounts", "broken.json"), "{")
	err := New(home).Switch("broken")
	if err == nil {
		t.Fatal("esperava erro para perfil corrompido")
	}
}

func TestSwitchActiveKeepsRefreshedCredentials(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"k":{"email":"work@example.com","refresh_token":"old"}}`)
	live := `{"k":{"email":"work@example.com","refresh_token":"renewed"}}`
	writeFile(t, filepath.Join(home, "auth.json"), live)
	if err := New(home).Switch("work"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != live {
		t.Fatal("switching the active account replaced refreshed credentials")
	}
}

func TestSwitchSnapshotsRefreshedCredentialsBeforeLeaving(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"k":{"email":"work@example.com","refresh_token":"old"}}`)
	writeProfile(t, home, "personal", "me@example.com", `{"k":{"email":"me@example.com"}}`)
	live := `{"k":{"email":"work@example.com","refresh_token":"renewed","unknown":"preserved"}}`
	writeFile(t, filepath.Join(home, "auth.json"), live)
	st := New(home)
	if err := st.Switch("personal"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, "accounts", "work.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved profile
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(saved.Auth, []byte(live)) {
		t.Fatalf("snapshot did not preserve renewed credentials: %s", saved.Auth)
	}
	if err := st.Switch("work"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(got, []byte(live)) {
		t.Fatal("round trip restored stale credentials")
	}
	info, err := os.Stat(filepath.Join(home, "accounts", "work.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal("snapshot permissions must be 0600")
	}
}

func jsonEqual(a, b []byte) bool {
	var x, y interface{}
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func TestListSyncsRenewedCredentialsWithoutRewritingUnchangedSnapshot(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"k":{"email":"work@example.com","refresh_token":"old"}}`)
	live := `{"k":{"email":"work@example.com","refresh_token":"new"}}`
	writeFile(t, filepath.Join(home, "auth.json"), live)
	st := New(home)
	if _, err := st.List(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "accounts", "work.json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved profile
	if err := json.Unmarshal(first, &saved); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(saved.Auth, []byte(live)) {
		t.Fatal("list did not sync renewed token")
	}
	if _, err := st.List(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("unchanged session rewrote saved_at")
	}
	if err := os.Remove(filepath.Join(home, "auth.json")); err != nil {
		t.Fatal(err)
	}
	entries, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Active {
		t.Fatal("missing session marked active")
	}
	third, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(third) {
		t.Fatal("missing auth destroyed saved credentials")
	}
}

func TestSwitchRejectsNullAndUnrefreshableExpiredToken(t *testing.T) {
	for _, auth := range []string{"null", `{"k":{"email":"different@example.com"}}`, `{}`, `{"k":{"email":"work@example.com","expires_at":"2000-01-01T00:00:00Z"}}`} {
		t.Run(auth, func(t *testing.T) {
			home := t.TempDir()
			writeProfile(t, home, "work", "work@example.com", auth)
			writeAuth(t, home, "me@example.com")
			before, err := os.ReadFile(filepath.Join(home, "auth.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := New(home).Switch("work"); err == nil {
				t.Fatal("invalid session accepted")
			}
			after, err := os.ReadFile(filepath.Join(home, "auth.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("failed switch changed active credentials")
			}
		})
	}
}

func TestExpiredRefreshableProfileWarnsAndStillSwitches(t *testing.T) {
	home := t.TempDir()
	writeProfile(t, home, "work", "work@example.com", `{"k":{"email":"work@example.com","expires_at":"2000-01-01T00:00:00Z","refresh_token":"refresh"}}`)
	st := New(home)
	entries, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Warning == "" {
		t.Fatal("missing renewal warning")
	}
	if err := st.Switch("work"); err != nil {
		t.Fatalf("refreshable token blocked: %v", err)
	}
}
