package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestAdd_createsInactiveAccount(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	name := gofakeit.LetterN(10)
	key := "sk-" + gofakeit.LetterN(40)
	if err := s.Add(name, key); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var found *Entry
	for i := range entries {
		if entries[i].Name == name {
			found = &entries[i]
		}
	}
	if found == nil {
		t.Fatalf("missing %q in %+v", name, entries)
	}
	if found.Active {
		t.Fatal("add must not activate")
	}
	if readAccount(t, fx.Dir).Active[serviceGo] != fx.GoID {
		t.Fatal("active id changed")
	}
	auth := readAuth(t, fx.Dir)
	var cred struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(auth[serviceGo], &cred)
	if cred.Key != fx.GoKey {
		t.Fatal("add must not write auth.json")
	}
}

func TestAdd_rejectsDuplicateName(t *testing.T) {
	fx := newFixture(t)
	err := New(fx.Dir).Add(fx.GoName, "sk-"+gofakeit.LetterN(40))
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("já existe conta Go chamada %q. Use outro nome ou remova a atual.", fx.GoName)
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestAdd_rejectsEmptyKey(t *testing.T) {
	fx := newFixture(t)
	err := New(fx.Dir).Add(gofakeit.LetterN(8), "  ")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "informe a chave: ocgs add <nome> <chave>" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestAdd_rejectsInvalidName(t *testing.T) {
	fx := newFixture(t)
	err := New(fx.Dir).Add("foo/bar", "sk-"+gofakeit.LetterN(40))
	if err == nil {
		t.Fatal("expected error")
	}
	want := `nome de conta inválido: "foo/bar". Use um nome sem "/" e não vazio.`
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestSave_renamesActive(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	newName := gofakeit.LetterN(10)
	if err := s.Save(newName); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != newName || !entries[0].Active {
		t.Fatalf("%+v", entries)
	}
}

func TestSave_sameNameIsNoop(t *testing.T) {
	fx := newFixture(t)
	if err := New(fx.Dir).Save(fx.GoName); err != nil {
		t.Fatal(err)
	}
}

func TestSave_withoutActive(t *testing.T) {
	fx := newFixture(t)
	acc := readAccount(t, fx.Dir)
	delete(acc.Active, serviceGo)
	writeJSON(t, filepath.Join(fx.Dir, "account.json"), acc)
	err := New(fx.Dir).Save(gofakeit.LetterN(8))
	if err == nil {
		t.Fatal("expected error")
	}
	want := "não há conta OpenCode Go ativa para salvar. Faça login ou ocgs add … e ocgs switch."
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestSave_rejectsDuplicateName(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	other := gofakeit.LetterN(10)
	if err := s.Add(other, "sk-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	err := s.Save(other)
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("já existe conta Go chamada %q. Use outro nome ou remova a atual.", other)
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestRemove_deletesInactive(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	extra := gofakeit.LetterN(10)
	if err := s.Add(extra, "sk-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(extra); err != nil {
		t.Fatal(err)
	}
	for _, e := range mustList(t, s) {
		if e.Name == extra {
			t.Fatalf("still listed %q", extra)
		}
	}
}

func TestRemove_rejectsActive(t *testing.T) {
	fx := newFixture(t)
	err := New(fx.Dir).Remove(fx.GoName)
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("%q está ativa. Troque com ocgs switch <outra> antes de remover.", fx.GoName)
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestRemove_duplicateName(t *testing.T) {
	fx := newFixture(t)
	acc := readAccount(t, fx.Dir)
	dupID := "01ARZ3NDEKTSV4RRFFQ69G5RMV"
	cred, _ := json.Marshal(map[string]string{"type": "api", "key": "sk-" + gofakeit.LetterN(40)})
	acc.Accounts[dupID] = Account{
		ID: dupID, ServiceID: serviceGo, Description: fx.GoName, Credential: cred,
	}
	writeJSON(t, filepath.Join(fx.Dir, "account.json"), acc)
	err := New(fx.Dir).Remove(fx.GoName)
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("há 2 contas Go chamadas %q. Renomeie a ativa com ocgs save <nome-único>.", fx.GoName)
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func mustList(t *testing.T, s *Store) []Entry {
	t.Helper()
	e, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestActiveGoKey_returnsKey(t *testing.T) {
	fx := newFixture(t)
	key, err := New(fx.Dir).ActiveGoKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != fx.GoKey {
		t.Fatalf("got %q", key)
	}
}

func TestActiveGoKey_missingActive(t *testing.T) {
	fx := newFixture(t)
	acc := readAccount(t, fx.Dir)
	delete(acc.Active, serviceGo)
	writeJSON(t, filepath.Join(fx.Dir, "account.json"), acc)
	_, err := New(fx.Dir).ActiveGoKey()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "não há chave Go ativa para verificar." {
		t.Fatalf("got %q", err.Error())
	}
}

func TestAdd_writes0600AndULID(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	name := gofakeit.LetterN(10)
	if err := s.Add(name, "sk-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(fx.Dir, "account.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm %o", info.Mode().Perm())
	}
	acc := readAccount(t, fx.Dir)
	var id string
	for _, a := range acc.Accounts {
		if a.Description == name {
			id = a.ID
		}
	}
	if len(id) != 26 {
		t.Fatalf("id %q len %d", id, len(id))
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			t.Fatalf("non-crockford %q in %q", c, id)
		}
	}
}
