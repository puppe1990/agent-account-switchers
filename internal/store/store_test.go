package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
)

const serviceGo = "opencode-go"

type fixture struct {
	Dir      string
	GoID     string
	GoName   string
	GoKey    string
	OtherID  string
	OtherKey string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	fx := fixture{
		Dir:      dir,
		GoID:     "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		GoName:   gofakeit.LetterN(10),
		GoKey:    "sk-" + gofakeit.LetterN(40),
		OtherID:  "01ARZ3NDEKTSV4RRFFQ69G5FB0",
		OtherKey: "sk-" + gofakeit.LetterN(40),
	}
	goCred, _ := json.Marshal(map[string]string{"type": "api", "key": fx.GoKey})
	otherCred, _ := json.Marshal(map[string]string{"type": "api", "key": fx.OtherKey})
	acc := File{
		Version: 2,
		Accounts: map[string]Account{
			fx.GoID: {
				ID: fx.GoID, ServiceID: serviceGo,
				Description: fx.GoName, Credential: goCred,
			},
			fx.OtherID: {
				ID: fx.OtherID, ServiceID: "openai",
				Description: "default", Credential: otherCred,
			},
		},
		Active: map[string]string{
			serviceGo: fx.GoID,
			"openai":  fx.OtherID,
		},
	}
	writeJSON(t, filepath.Join(dir, "account.json"), acc)
	auth := map[string]json.RawMessage{
		serviceGo: goCred,
		"openai":  otherCred,
	}
	writeJSON(t, filepath.Join(dir, "auth.json"), auth)
	return fx
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readAccount(t *testing.T, dir string) File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "account.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func readAuth(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestList_missingStore(t *testing.T) {
	dir := t.TempDir()
	_, err := New(dir).List()
	if err == nil {
		t.Fatal("expected error")
	}
	want := "não achei o store do OpenCode em " + dir + ". Abra o OpenCode uma vez ou rode opencode auth login."
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestList_invalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "account.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := New(dir).List()
	if err == nil {
		t.Fatal("expected error")
	}
	if wantPrefix := "account.json em " + path + " não é um JSON v2 válido:"; len(err.Error()) < len(wantPrefix) || err.Error()[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("got %q", err.Error())
	}
}
