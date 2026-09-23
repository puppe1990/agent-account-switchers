package cccli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"opencode-go-switcher/internal/ccstore"
)

type stubVerifier struct{ err error }

func (s stubVerifier) Verify(string) error { return s.err }

func writeCLIFixture(t *testing.T) (dir, name, key string) {
	t.Helper()
	dir = t.TempDir()
	name = gofakeit.LetterN(10)
	key = "user-" + gofakeit.LetterN(40)
	auth := ccstore.Auth{APIKey: key, UserName: "puppe1990", KeyName: "cli-x"}
	b, _ := json.MarshalIndent(auth, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "auth.json"), b, 0o600)
	ledger := ccstore.Ledger{
		Version:  1,
		Accounts: map[string]ccstore.Auth{name: auth},
		Active:   name,
	}
	lb, _ := json.MarshalIndent(ledger, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "ccs-accounts.json"), lb, 0o600)
	return dir, name, key
}

func runCLI(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd := New(ccstore.New(dir), stubVerifier{}, out, errBuf)
	cmd.SetArgs(NormalizeArgs(args))
	err = cmd.Execute()
	return out.String(), errBuf.String(), err
}

func TestCLI_listMarksActiveOmitsKey(t *testing.T) {
	dir, name, key := writeCLIFixture(t)
	stdout, _, err := runCLI(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "→ "+name) {
		t.Fatalf("stdout %q", stdout)
	}
	if strings.Contains(stdout, key) {
		t.Fatal("leaked key")
	}
}

func TestCLI_listSubcommand(t *testing.T) {
	dir, name, _ := writeCLIFixture(t)
	stdout, _, err := runCLI(t, dir, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, name) {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestCLI_switchSubcommandAndPositional(t *testing.T) {
	dir, _, _ := writeCLIFixture(t)
	s := ccstore.New(dir)
	other := gofakeit.LetterN(10)
	if err := s.Add(other, "user-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, "switch", other); err != nil {
		t.Fatal(err)
	}
	active := activeName(t, s)
	if active != other {
		t.Fatalf("active %q", active)
	}
	third := gofakeit.LetterN(10)
	if err := s.Add(third, "user-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, third); err != nil {
		t.Fatal(err)
	}
	if activeName(t, s) != third {
		t.Fatalf("positional active %q", activeName(t, s))
	}
}

func TestCLI_switchWithoutName(t *testing.T) {
	dir, _, _ := writeCLIFixture(t)
	_, _, err := runCLI(t, dir, "switch")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "informe a conta: ccs switch <nome>" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestCLI_addSaveRemove(t *testing.T) {
	dir, name, _ := writeCLIFixture(t)
	s := ccstore.New(dir)
	added := gofakeit.LetterN(10)
	key := "user-" + gofakeit.LetterN(40)
	if _, _, err := runCLI(t, dir, "add", added, key); err != nil {
		t.Fatal(err)
	}
	renamed := gofakeit.LetterN(10)
	if _, _, err := runCLI(t, dir, "save", renamed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, "remove", added); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.List()
	foundRenamed := false
	for _, e := range entries {
		if e.Name == added {
			t.Fatal("remove failed")
		}
		if e.Name == renamed {
			foundRenamed = true
		}
	}
	if !foundRenamed {
		t.Fatalf("save missing %q in %+v (original %q)", renamed, entries, name)
	}
}

func TestCLI_verifyOKAnd401(t *testing.T) {
	dir, _, _ := writeCLIFixture(t)
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd := New(ccstore.New(dir), stubVerifier{}, out, errBuf)
	cmd.SetArgs([]string{"verify"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "chave Command Code ativa é válida.") {
		t.Fatalf("stdout %q", out.String())
	}
	cmd = New(ccstore.New(dir), stubVerifier{err: fmt.Errorf("a chave ativa foi recusada pela API Command Code (HTTP 401). Troque de conta ou gere outra chave.")}, out, errBuf)
	cmd.SetArgs([]string{"verify"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("got %q", err.Error())
	}
	if strings.Contains(err.Error(), "user-") {
		t.Fatal("leaked key")
	}
}

func TestCLI_missingAuthOnSave(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runCLI(t, dir, "save", "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "não achei o auth do Command Code em "+dir) {
		t.Fatalf("got %q", err.Error())
	}
}

func activeName(t *testing.T, s *ccstore.Store) string {
	t.Helper()
	entries, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Active {
			return e.Name
		}
	}
	return ""
}
