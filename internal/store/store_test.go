package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
)

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

func TestList_marksActiveAndOmitsKey(t *testing.T) {
	fx := newFixture(t)
	entries, err := New(fx.Dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries: %+v", len(entries), entries)
	}
	if entries[0].Name != fx.GoName {
		t.Fatalf("name %q", entries[0].Name)
	}
	if !entries[0].Active {
		t.Fatal("expected active")
	}
	raw, err := os.ReadFile(filepath.Join(fx.Dir, "account.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("fixture broken")
	}
	for _, e := range entries {
		if e.Name == fx.GoKey || e.Name == fx.OtherKey {
			t.Fatalf("listed a key: %q", e.Name)
		}
	}
}

func TestSwitch_updatesActiveAndAuthJSON(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	second := gofakeit.LetterN(10)
	secondKey := "sk-" + gofakeit.LetterN(40)
	if err := s.Add(second, secondKey); err != nil {
		t.Fatal(err)
	}
	if err := s.Switch(second); err != nil {
		t.Fatal(err)
	}
	acc := readAccount(t, fx.Dir)
	var switched Account
	for _, a := range acc.Accounts {
		if a.ServiceID == serviceGo && a.Description == second {
			switched = a
		}
	}
	if acc.Active[serviceGo] != switched.ID {
		t.Fatalf("active=%s want %s", acc.Active[serviceGo], switched.ID)
	}
	auth := readAuth(t, fx.Dir)
	var cred struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(auth[serviceGo], &cred); err != nil {
		t.Fatal(err)
	}
	if cred.Key != secondKey {
		t.Fatalf("auth key %q", cred.Key)
	}
}

func TestSwitch_preservesOtherProviders(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	other := gofakeit.LetterN(10)
	if err := s.Add(other, "sk-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	if err := s.Switch(other); err != nil {
		t.Fatal(err)
	}
	acc := readAccount(t, fx.Dir)
	if acc.Active["openai"] != fx.OtherID {
		t.Fatalf("openai active mutated: %s", acc.Active["openai"])
	}
	auth := readAuth(t, fx.Dir)
	var cred struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(auth["openai"], &cred); err != nil {
		t.Fatal(err)
	}
	if cred.Key != fx.OtherKey {
		t.Fatalf("openai auth mutated")
	}
}

func TestSwitch_unknownName(t *testing.T) {
	fx := newFixture(t)
	err := New(fx.Dir).Switch("nao-existe")
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("conta %q não existe. Contas Go: %s.", "nao-existe", fx.GoName)
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestSwitch_duplicateName(t *testing.T) {
	fx := newFixture(t)
	acc := readAccount(t, fx.Dir)
	dupID := "01ARZ3NDEKTSV4RRFFQ69G5DUP"
	cred, _ := json.Marshal(map[string]string{"type": "api", "key": "sk-" + gofakeit.LetterN(40)})
	acc.Accounts[dupID] = Account{
		ID: dupID, ServiceID: serviceGo, Description: fx.GoName, Credential: cred,
	}
	writeJSON(t, filepath.Join(fx.Dir, "account.json"), acc)
	err := New(fx.Dir).Switch(fx.GoName)
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("há 2 contas Go chamadas %q. Renomeie a ativa com ocgs save <nome-único>.", fx.GoName)
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}
