package grokstore

import (
	"os"
	"path/filepath"
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
	writeProfile(t, home, "work", "Work@Example.com", `{"token":"WORK"}`)
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
