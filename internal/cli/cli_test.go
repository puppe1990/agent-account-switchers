package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"agent-account-switchers/internal/store"
)

type stubVerifier struct{ err error }

func (s stubVerifier) Verify(string) error { return s.err }

func writeCLIFixture(t *testing.T) (dir, goName, goKey string) {
	t.Helper()
	dir = t.TempDir()
	goName = gofakeit.LetterN(10)
	goKey = "sk-" + gofakeit.LetterN(40)
	goID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	otherID := "01ARZ3NDEKTSV4RRFFQ69G5FB0"
	goCred, _ := json.Marshal(map[string]string{"type": "api", "key": goKey})
	otherCred, _ := json.Marshal(map[string]string{"type": "api", "key": "sk-" + gofakeit.LetterN(40)})
	acc := store.File{
		Version: 2,
		Accounts: map[string]store.Account{
			goID:    {ID: goID, ServiceID: "opencode-go", Description: goName, Credential: goCred},
			otherID: {ID: otherID, ServiceID: "openai", Description: "default", Credential: otherCred},
		},
		Active: map[string]string{"opencode-go": goID, "openai": otherID},
	}
	b, _ := json.MarshalIndent(acc, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "account.json"), b, 0o600)
	ab, _ := json.MarshalIndent(map[string]json.RawMessage{"opencode-go": goCred, "openai": otherCred}, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "auth.json"), ab, 0o600)
	return dir, goName, goKey
}

func runCLI(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd := New(store.New(dir), stubVerifier{}, out, errBuf)
	cmd.SetArgs(NormalizeArgs(args))
	err = cmd.Execute()
	return out.String(), errBuf.String(), err
}

func TestCLI_listMarksActiveOmitsKey(t *testing.T) {
	dir, goName, goKey := writeCLIFixture(t)
	stdout, _, err := runCLI(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "→ "+goName) {
		t.Fatalf("stdout %q", stdout)
	}
	if strings.Contains(stdout, goKey) {
		t.Fatal("leaked key")
	}
}

func TestCLI_listSubcommand(t *testing.T) {
	dir, goName, _ := writeCLIFixture(t)
	stdout, _, err := runCLI(t, dir, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, goName) {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestCLI_switchSubcommandAndPositional(t *testing.T) {
	dir, _, _ := writeCLIFixture(t)
	s := store.New(dir)
	other := gofakeit.LetterN(10)
	if err := s.Add(other, "sk-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, "switch", other); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.List()
	var active string
	for _, e := range entries {
		if e.Active {
			active = e.Name
		}
	}
	if active != other {
		t.Fatalf("active %q", active)
	}
	third := gofakeit.LetterN(10)
	if err := s.Add(third, "sk-"+gofakeit.LetterN(40)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, third); err != nil {
		t.Fatal(err)
	}
	entries, _ = s.List()
	for _, e := range entries {
		if e.Active {
			active = e.Name
		}
	}
	if active != third {
		t.Fatalf("positional active %q", active)
	}
}

func TestCLI_switchWithoutName(t *testing.T) {
	dir, _, _ := writeCLIFixture(t)
	_, _, err := runCLI(t, dir, "switch")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "informe a conta: ocgs switch <nome>" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestCLI_addSaveRemove(t *testing.T) {
	dir, goName, _ := writeCLIFixture(t)
	s := store.New(dir)
	added := gofakeit.LetterN(10)
	key := "sk-" + gofakeit.LetterN(40)
	if _, _, err := runCLI(t, dir, "add", added, key); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.List()
	var addedActive bool
	found := false
	for _, e := range entries {
		if e.Name == added {
			found = true
			addedActive = e.Active
		}
	}
	if !found || addedActive {
		t.Fatalf("add %+v", entries)
	}
	renamed := gofakeit.LetterN(10)
	if _, _, err := runCLI(t, dir, "save", renamed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, dir, "remove", added); err != nil {
		t.Fatal(err)
	}
	entries, _ = s.List()
	for _, e := range entries {
		if e.Name == added {
			t.Fatal("remove failed")
		}
		if e.Name == goName {
			t.Fatal("save should have renamed active")
		}
	}
}

func TestCLI_verifyOKAnd401(t *testing.T) {
	dir, _, _ := writeCLIFixture(t)
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd := New(store.New(dir), stubVerifier{}, out, errBuf)
	cmd.SetArgs([]string{"verify"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "chave OpenCode Go ativa é válida.") {
		t.Fatalf("stdout %q", out.String())
	}

	out.Reset()
	cmd = New(store.New(dir), stubVerifier{err: fmt.Errorf("a chave ativa foi recusada pela API OpenCode Go (HTTP 401). Troque de conta ou gere outra chave.")}, out, errBuf)
	cmd.SetArgs([]string{"verify"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("got %q", err.Error())
	}
	if strings.Contains(err.Error(), "sk-") {
		t.Fatal("leaked key")
	}
}

func TestCLI_missingStore(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runCLI(t, dir, "list")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "não achei o store do OpenCode em "+dir) {
		t.Fatalf("got %q", err.Error())
	}
}
