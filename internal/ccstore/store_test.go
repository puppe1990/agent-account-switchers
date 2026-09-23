package ccstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

type fixture struct {
	Dir  string
	Name string
	Key  string
	Auth Auth
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	name := gofakeit.LetterN(10)
	key := "user-" + gofakeit.LetterN(40)
	auth := Auth{
		APIKey:          key,
		UserID:          gofakeit.UUID(),
		UserName:        gofakeit.Username(),
		KeyName:         "cli-" + gofakeit.LetterN(8),
		AuthenticatedAt: "2026-05-23T21:23:45.149Z",
	}
	writeJSON(t, filepath.Join(dir, "auth.json"), auth)
	ledger := Ledger{
		Version:  1,
		Accounts: map[string]Auth{name: auth},
		Active:   name,
	}
	writeJSON(t, filepath.Join(dir, "ccs-accounts.json"), ledger)
	return fixture{Dir: dir, Name: name, Key: key, Auth: auth}
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

func readAuth(t *testing.T, dir string) Auth {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var a Auth
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func readLedger(t *testing.T, dir string) Ledger {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "ccs-accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var l Ledger
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestList_marksActiveAndOmitsKey(t *testing.T) {
	fx := newFixture(t)
	entries, err := New(fx.Dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %+v", entries)
	}
	if entries[0].Name != fx.Name || !entries[0].Active {
		t.Fatalf("%+v", entries[0])
	}
	if entries[0].Name == fx.Key {
		t.Fatal("listed a key")
	}
}

func TestList_emptyLedger(t *testing.T) {
	dir := t.TempDir()
	entries, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %+v", entries)
	}
}

func TestSwitch_updatesAuthJSON(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	second := gofakeit.LetterN(10)
	secondKey := "user-" + gofakeit.LetterN(40)
	if err := s.Add(second, secondKey); err != nil {
		t.Fatal(err)
	}
	if err := s.Switch(second); err != nil {
		t.Fatal(err)
	}
	auth := readAuth(t, fx.Dir)
	if auth.APIKey != secondKey {
		t.Fatalf("auth key %q", auth.APIKey)
	}
	if readLedger(t, fx.Dir).Active != second {
		t.Fatal("active pointer")
	}
}

func TestSwitch_unknownName(t *testing.T) {
	fx := newFixture(t)
	err := New(fx.Dir).Switch("nao-existe")
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("conta %q não existe. Contas Command Code: %s.", "nao-existe", fx.Name)
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestAdd_createsInactive(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	name := gofakeit.LetterN(10)
	key := "user-" + gofakeit.LetterN(40)
	if err := s.Add(name, key); err != nil {
		t.Fatal(err)
	}
	if readAuth(t, fx.Dir).APIKey != fx.Key {
		t.Fatal("add must not write auth.json")
	}
	ok := false
	for _, e := range mustList(t, s) {
		if e.Name == name {
			ok = true
			if e.Active {
				t.Fatal("add must not activate")
			}
		}
	}
	if !ok {
		t.Fatal("missing added account")
	}
}

func TestAdd_rejectsDuplicateAndEmptyAndInvalid(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	err := s.Add(fx.Name, "user-"+gofakeit.LetterN(8))
	if err == nil || err.Error() != fmt.Sprintf("já existe conta Command Code chamada %q. Use outro nome ou remova a atual.", fx.Name) {
		t.Fatalf("dup %v", err)
	}
	err = s.Add(gofakeit.LetterN(8), "  ")
	if err == nil || err.Error() != "informe a chave: ccs add <nome> <chave>" {
		t.Fatalf("empty %v", err)
	}
	err = s.Add("foo/bar", "user-x")
	want := `nome de conta inválido: "foo/bar". Use um nome sem "/" e não vazio.`
	if err == nil || err.Error() != want {
		t.Fatalf("invalid %v", err)
	}
}

func TestSave_snapshotsActive(t *testing.T) {
	dir := t.TempDir()
	key := "user-" + gofakeit.LetterN(40)
	auth := Auth{APIKey: key, UserName: "puppe1990", KeyName: "cli-x", AuthenticatedAt: "2026-01-01T00:00:00Z"}
	writeJSON(t, filepath.Join(dir, "auth.json"), auth)
	s := New(dir)
	name := gofakeit.LetterN(10)
	if err := s.Save(name); err != nil {
		t.Fatal(err)
	}
	l := readLedger(t, dir)
	if l.Accounts[name].APIKey != key {
		t.Fatalf("%+v", l.Accounts[name])
	}
	if err := s.Save(name); err != nil {
		t.Fatal(err)
	}
}

func TestSave_withoutAuth(t *testing.T) {
	dir := t.TempDir()
	err := New(dir).Save(gofakeit.LetterN(8))
	if err == nil {
		t.Fatal("expected error")
	}
	want := "não achei o auth do Command Code em " + dir + ". Rode cmd login."
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestSave_rejectsDuplicateDifferentKey(t *testing.T) {
	fx := newFixture(t)
	otherKey := "user-" + gofakeit.LetterN(40)
	writeJSON(t, filepath.Join(fx.Dir, "auth.json"), Auth{APIKey: otherKey, UserName: "x"})
	err := New(fx.Dir).Save(fx.Name)
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("já existe conta Command Code chamada %q. Use outro nome ou remova a atual.", fx.Name)
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestRemove_deletesInactiveAndRejectsActive(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	extra := gofakeit.LetterN(10)
	if err := s.Add(extra, "user-"+gofakeit.LetterN(40)); err != nil {
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
	err := s.Remove(fx.Name)
	if err == nil {
		t.Fatal("expected error")
	}
	want := fmt.Sprintf("%q está ativa. Troque com ccs switch <outra> antes de remover.", fx.Name)
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestActiveKey_and0600(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	key, err := s.ActiveKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != fx.Key {
		t.Fatalf("got %q", key)
	}
	name := gofakeit.LetterN(8)
	if err := s.Add(name, "user-"+gofakeit.LetterN(12)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(fx.Dir, "ccs-accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm %o", info.Mode().Perm())
	}
}

func TestActiveKey_missing(t *testing.T) {
	dir := t.TempDir()
	_, err := New(dir).ActiveKey()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "não há chave Command Code ativa para verificar." {
		t.Fatalf("got %q", err.Error())
	}
}

func TestAdd_setsManualMetadata(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	s.Now = func() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) }
	name := gofakeit.LetterN(8)
	if err := s.Add(name, "user-abc"); err != nil {
		t.Fatal(err)
	}
	acc := readLedger(t, fx.Dir).Accounts[name]
	if acc.KeyName != "cli-manual-entry" || acc.UserName != name {
		t.Fatalf("%+v", acc)
	}
	if acc.AuthenticatedAt != "2026-09-23T12:00:00Z" {
		t.Fatalf("ts %q", acc.AuthenticatedAt)
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
