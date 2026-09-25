# OpenCode Go Switcher (`ocgs`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `ocgs` CLI that lists, adds, saves, switches, removes, and verifies OpenCode Go accounts in the native OpenCode 1.18 `account.json` v2 store (and syncs `auth.json` on switch).

**Architecture:** Domain `Store` owns `account.json` + `auth.json` behind a constructor that receives the data directory. Cobra CLI is a thin adapter (`ocgs <nome>` is rewritten to `switch <nome>`). `verify` is an injectable HTTP client hitting `/zen/go/v1/models`. Tests never touch the real HOME: they pass `t.TempDir()` and `httptest.Server`.

**Tech Stack:** Go 1.24, Cobra, stdlib `testing`, `gofakeit/v7`, `crypto/rand` ULID, `net/http/httptest`.

**Spec:** `docs/superpowers/specs/2026-09-23-agent-account-switchers-design.md`

---

## File map

| File                             | Responsibility                                                        |
| -------------------------------- | --------------------------------------------------------------------- |
| `go.mod` / `go.sum`              | Module `agent-account-switchers`, Cobra + gofakeit                    |
| `.gitignore`                     | Binaries, `.DS_Store`                                                 |
| `internal/store/paths.go`        | `ResolveDataDir(getenv, home)`                                        |
| `internal/store/id.go`           | `newID()` ULID 26 chars                                               |
| `internal/store/store.go`        | Load/save, List/Switch/Add/Save/Remove, name rules, atomic 0600 write |
| `internal/store/paths_test.go`   | Path resolution                                                       |
| `internal/store/store_test.go`   | All store behaviors + faker fixtures                                  |
| `internal/verify/verify.go`      | `Client.Verify(key)`                                                  |
| `internal/verify/verify_test.go` | httptest 2xx / 401 / network                                          |
| `internal/cli/cli.go`            | Cobra commands, arg rewrite, stdout/stderr                            |
| `internal/cli/cli_test.go`       | CLI behaviors against temp store                                      |
| `cmd/ocgs/main.go`               | Wire env, home, HTTP timeout, `os.Exit(1)`                            |
| `README.md`                      | Install, commands, “sessões abertas não mudam”                        |

Constants used everywhere:

- service id: `"opencode-go"`
- files: `account.json`, `auth.json`
- default verify base: `https://opencode.ai/zen/go/v1`

Store types (lock these names; later tasks reuse them):

```go
type Account struct {
	ID          string          `json:"id"`
	ServiceID   string          `json:"serviceID"`
	Description string          `json:"description"`
	Credential  json.RawMessage `json:"credential"`
}

type File struct {
	Version  int                `json:"version"`
	Accounts map[string]Account `json:"accounts"`
	Active   map[string]string  `json:"active"`
}

type Entry struct {
	Name   string
	Active bool
}

type Store struct {
	Dir string
}

func New(dir string) *Store
func ResolveDataDir(getenv func(string) string, home string) string
func (s *Store) List() ([]Entry, error)
func (s *Store) Switch(name string) error
func (s *Store) Add(name, key string) error
func (s *Store) Save(name string) error
func (s *Store) Remove(name string) error
func (s *Store) ActiveGoKey() (string, error)
```

`Credential` of non-Go accounts stays `json.RawMessage` so OAuth fields survive a rewrite.

---

### Task 1: Scaffold module

**Files:**

- Create: `go.mod`
- Create: `.gitignore`

- [ ] **Step 1: Init module and gitignore**

```bash
go mod init agent-account-switchers
go get github.com/spf13/cobra@v1.10.1
go get github.com/brianvoe/gofakeit/v7@v7.4.0
```

`.gitignore`:

```
/ocgs
/bin/
.DS_Store
*.exe
```

- [ ] **Step 2: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: init Go module for ocgs"
```

---

### Task 2: Resolve data dir

**Files:**

- Create: `internal/store/paths.go`
- Create: `internal/store/paths_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package store

import "testing"

func TestResolveDataDir_openCodeDataDirWins(t *testing.T) {
	env := map[string]string{
		"OPENCODE_DATA_DIR": "/tmp/custom-opencode",
		"XDG_DATA_HOME":     "/tmp/xdg",
	}
	got := ResolveDataDir(func(k string) string { return env[k] }, "/home/dev")
	if got != "/tmp/custom-opencode" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveDataDir_xdgThenHome(t *testing.T) {
	got := ResolveDataDir(func(k string) string {
		if k == "XDG_DATA_HOME" {
			return "/tmp/xdg"
		}
		return ""
	}, "/home/dev")
	if got != "/tmp/xdg/opencode" {
		t.Fatalf("got %q", got)
	}

	got = ResolveDataDir(func(string) string { return "" }, "/home/dev")
	if got != "/home/dev/.local/share/opencode" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run ResolveDataDir -count=1`

Expected: FAIL with `undefined: ResolveDataDir`

- [ ] **Step 3: Write minimal implementation**

```go
package store

import "path/filepath"

func ResolveDataDir(getenv func(string) string, home string) string {
	if dir := getenv("OPENCODE_DATA_DIR"); dir != "" {
		return dir
	}
	if xdg := getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	return filepath.Join(home, ".local", "share", "opencode")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run ResolveDataDir -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/paths.go internal/store/paths_test.go
git commit -m "feat: resolve OpenCode data dir from env and home"
```

---

### Task 3: Load store + fixture helper

**Files:**

- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
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
		t.fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.fatal(err)
	}
}

func readAccount(t *testing.T, dir string) File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "account.json"))
	if err != nil {
		t.fatal(err)
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		t.fatal(err)
	}
	return f
}

func readAuth(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.fatal(err)
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
		t.fatal(err)
	}
	_, err := New(dir).List()
	if err == nil {
		t.Fatal("expected error")
	}
	if wantPrefix := "account.json em " + path + " não é um JSON v2 válido:"; len(err.Error()) < len(wantPrefix) || err.Error()[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("got %q", err.Error())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestList_missingStore|TestList_invalidJSON' -count=1`

Expected: FAIL with `undefined: New` / `undefined: File` / `undefined: Account`

- [ ] **Step 3: Write minimal implementation**

```go
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Account struct {
	ID          string          `json:"id"`
	ServiceID   string          `json:"serviceID"`
	Description string          `json:"description"`
	Credential  json.RawMessage `json:"credential"`
}

type File struct {
	Version  int                `json:"version"`
	Accounts map[string]Account `json:"accounts"`
	Active   map[string]string  `json:"active"`
}

type Entry struct {
	Name   string
	Active bool
}

type Store struct {
	Dir string
}

func New(dir string) *Store {
	return &Store{Dir: dir}
}

func (s *Store) accountPath() string { return filepath.Join(s.Dir, "account.json") }
func (s *Store) authPath() string    { return filepath.Join(s.Dir, "auth.json") }

func (s *Store) missing() error {
	return fmt.Errorf("não achei o store do OpenCode em %s. Abra o OpenCode uma vez ou rode opencode auth login.", s.Dir)
}

func (s *Store) load() (*File, error) {
	b, err := os.ReadFile(s.accountPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, s.missing()
		}
		return nil, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("account.json em %s não é um JSON v2 válido: %v", s.accountPath(), err)
	}
	if f.Version != 2 {
		return nil, fmt.Errorf("account.json em %s não é um JSON v2 válido: version %d", s.accountPath(), f.Version)
	}
	if f.Accounts == nil {
		f.Accounts = map[string]Account{}
	}
	if f.Active == nil {
		f.Active = map[string]string{}
	}
	return &f, nil
}

func (s *Store) List() ([]Entry, error) {
	_, err := s.load()
	if err != nil {
		return nil, err
	}
	return nil, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestList_missingStore|TestList_invalidJSON' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: load OpenCode account.json with missing and invalid errors"
```

---

### Task 4: List Go accounts

**Files:**

- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestList_marksActiveAndOmitsKey(t *testing.T) {
	fx := newFixture(t)
	entries, err := New(fx.Dir).List()
	if err != nil {
		t.fatal(err)
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
		t.fatal(err)
	}
	if !json.Valid(raw) {
		t.fatal("fixture broken")
	}
	for _, e := range entries {
		if e.Name == fx.GoKey || e.Name == fx.OtherKey {
			t.Fatalf("listed a key: %q", e.Name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestList_marksActiveAndOmitsKey -count=1`

Expected: FAIL `got 0 entries`

- [ ] **Step 3: Write minimal implementation**

Replace `List` in `internal/store/store.go` (add `"sort"` to imports):

```go
func (s *Store) List() ([]Entry, error) {
	f, err := s.load()
	if err != nil {
		return nil, err
	}
	activeID := f.Active[serviceGo]
	var entries []Entry
	for _, acc := range f.Accounts {
		if acc.ServiceID != serviceGo {
			continue
		}
		entries = append(entries, Entry{Name: acc.Description, Active: acc.ID == activeID})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}
```

Add `const serviceGo = "opencode-go"` to `store.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: list OpenCode Go accounts with active marker"
```

---

### Task 5: Switch updates active + auth, preserves others

**Files:**

- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`
- Create: `internal/store/id.go` (only if Add is needed; switch uses existing IDs — skip id.go here)

- [ ] **Step 1: Write the failing tests**

```go
func TestSwitch_updatesActiveAndAuthJSON(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	second := gofakeit.LetterN(10)
	secondKey := "sk-" + gofakeit.LetterN(40)
	if err := s.Add(second, secondKey); err != nil {
		t.fatal(err)
	}
	if err := s.Switch(second); err != nil {
		t.fatal(err)
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
		t.fatal(err)
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
		t.fatal(err)
	}
	if err := s.Switch(other); err != nil {
		t.fatal(err)
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
		t.fatal(err)
	}
	if cred.Key != fx.OtherKey {
		t.Fatalf("openai auth mutated")
	}
}
```

These tests call `Add` + `Switch`. Implement **both** in this task so the tests can run (Add is the smallest way to insert a second Go account). Keep Add incomplete vs later duplicate tests: here it only needs to insert an inactive Go account.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestSwitch_updatesActiveAndAuthJSON|TestSwitch_preservesOtherProviders' -count=1`

Expected: FAIL `s.Add undefined` or `s.Switch undefined`

- [ ] **Step 3: Write minimal implementation**

`internal/store/id.go`:

```go
package store

import (
	"crypto/rand"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newID() (string, error) {
	var raw [16]byte
	ms := uint64(time.Now().UnixMilli())
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	if _, err := rand.Read(raw[6:]); err != nil {
		return "", err
	}
	return encodeULID(raw), nil
}

func encodeULID(id [16]byte) string {
	dst := make([]byte, 26)
	dst[0] = crockford[(id[0]&224)>>5]
	dst[1] = crockford[id[0]&31]
	dst[2] = crockford[(id[1]&248)>>3]
	dst[3] = crockford[((id[1]&7)<<2)|((id[2]&192)>>6)]
	dst[4] = crockford[(id[2]&62)>>1]
	dst[5] = crockford[((id[2]&1)<<4)|((id[3]&240)>>4)]
	dst[6] = crockford[((id[3]&15)<<1)|((id[4]&128)>>7)]
	dst[7] = crockford[(id[4]&124)>>2]
	dst[8] = crockford[((id[4]&3)<<3)|((id[5]&224)>>5)]
	dst[9] = crockford[id[5]&31]
	dst[10] = crockford[(id[6]&248)>>3]
	dst[11] = crockford[((id[6]&7)<<2)|((id[7]&192)>>6)]
	dst[12] = crockford[(id[7]&62)>>1]
	dst[13] = crockford[((id[7]&1)<<4)|((id[8]&240)>>4)]
	dst[14] = crockford[((id[8]&15)<<1)|((id[9]&128)>>7)]
	dst[15] = crockford[(id[9]&124)>>2]
	dst[16] = crockford[((id[9]&3)<<3)|((id[10]&224)>>5)]
	dst[17] = crockford[id[10]&31]
	dst[18] = crockford[(id[11]&248)>>3]
	dst[19] = crockford[((id[11]&7)<<2)|((id[12]&192)>>6)]
	dst[20] = crockford[(id[12]&62)>>1]
	dst[21] = crockford[((id[12]&1)<<4)|((id[13]&240)>>4)]
	dst[22] = crockford[((id[13]&15)<<1)|((id[14]&128)>>7)]
	dst[23] = crockford[(id[14]&124)>>2]
	dst[24] = crockford[((id[14]&3)<<3)|((id[15]&224)>>5)]
	dst[25] = crockford[id[15]&31]
	return string(dst)
}
```

`id.go` imports: `crypto/rand`, `time`.

Add to `store.go`:

```go
func writeAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("não consegui gravar %s: %v. Nada foi alterado.", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("não consegui gravar %s: %v. Nada foi alterado.", path, err)
	}
	return nil
}

func (s *Store) loadAuth() (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(s.authPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, s.missing()
		}
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	return m, nil
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("nome de conta inválido: %q. Use um nome sem \"/\" e não vazio.", name)
	}
	return name, nil
}

func (s *Store) goByName(f *File, name string) []Account {
	var found []Account
	for _, acc := range f.Accounts {
		if acc.ServiceID == serviceGo && acc.Description == name {
			found = append(found, acc)
		}
	}
	return found
}

func (s *Store) goNames(f *File) []string {
	var names []string
	for _, acc := range f.Accounts {
		if acc.ServiceID == serviceGo {
			names = append(names, acc.Description)
		}
	}
	sort.Strings(names)
	return names
}

func (s *Store) notFound(name string, f *File) error {
	names := s.goNames(f)
	if len(names) == 0 {
		return fmt.Errorf("conta %q não existe. não há contas Go", name)
	}
	return fmt.Errorf("conta %q não existe. Contas Go: %s.", name, strings.Join(names, ", "))
}

func (s *Store) Add(name, key string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("informe a chave: ocgs add <nome> <chave>")
	}
	f, err := s.load()
	if err != nil {
		return err
	}
	if len(s.goByName(f, name)) > 0 {
		return fmt.Errorf("já existe conta Go chamada %q. Use outro nome ou remova a atual.", name)
	}
	id, err := newID()
	if err != nil {
		return err
	}
	cred, err := json.Marshal(map[string]string{"type": "api", "key": key})
	if err != nil {
		return err
	}
	f.Accounts[id] = Account{
		ID: id, ServiceID: serviceGo, Description: name, Credential: cred,
	}
	return writeAtomic(s.accountPath(), f)
}

func (s *Store) Switch(name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	f, err := s.load()
	if err != nil {
		return err
	}
	found := s.goByName(f, name)
	if len(found) == 0 {
		return s.notFound(name, f)
	}
	if len(found) > 1 {
		return fmt.Errorf("há %d contas Go chamadas %q. Renomeie a ativa com ocgs save <nome-único>.", len(found), name)
	}
	acc := found[0]
	f.Active[serviceGo] = acc.ID
	auth, err := s.loadAuth()
	if err != nil {
		return err
	}
	auth[serviceGo] = acc.Credential
	if err := writeAtomic(s.accountPath(), f); err != nil {
		return err
	}
	return writeAtomic(s.authPath(), auth)
}
```

Imports on `store.go`: `encoding/json`, `fmt`, `os`, `path/filepath`, `sort`, `strings`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/id.go internal/store/store_test.go
git commit -m "feat: switch OpenCode Go account and preserve other providers"
```

---

### Task 6: Switch errors (unknown + duplicate)

**Files:**

- Modify: `internal/store/store_test.go`
- Modify: `internal/store/store.go` (only if messages differ)

- [ ] **Step 1: Write the failing tests**

```go
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
```

Add `"fmt"` to `store_test.go` imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestSwitch_unknownName|TestSwitch_duplicateName' -count=1`

Expected: FAIL on message mismatch if `há %d` vs `há 2`, or PASS if Task 5 already used this exact string.

If Task 5 used `há %d contas Go chamadas` with `%d=2`, `TestSwitch_duplicateName` PASSES immediately — **that is not TDD**. Write `TestSwitch_unknownName` first in Task 5? Too late.

**If duplicate test passes immediately:** keep it as a regression test and still run Step 2; only add implementation if it fails. The unknown-name test should already pass from Task 5 `notFound`. If both pass immediately, do **not** write new production code; commit the tests as coverage.

- [ ] **Step 3: Align duplicate message to spec (`há 2`)**

In `Switch`, when `len(found) > 1`:

```go
return fmt.Errorf("há %d contas Go chamadas %q. Renomeie a ativa com ocgs save <nome-único>.", len(found), name)
```

This matches the test.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "test: reject unknown and duplicate Go account names on switch"
```

---

### Task 7: Add duplicate, empty key, invalid name, inactive

**Files:**

- Modify: `internal/store/store_test.go`
- Modify: `internal/store/store.go` if needed

- [ ] **Step 1: Write the failing tests**

```go
func TestAdd_createsInactiveAccount(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	name := gofakeit.LetterN(10)
	key := "sk-" + gofakeit.LetterN(40)
	if err := s.Add(name, key); err != nil {
		t.fatal(err)
	}
	entries, err := s.List()
	if err != nil {
		t.fatal(err)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestAdd_' -count=1`

Expected: FAIL on the first assertion that production code does not satisfy. If Task 5 Add already matches, tests PASS immediately — then **do not** change production code; commit tests.

- [ ] **Step 3: Fill gaps only**

`Add` from Task 5 already: normalize, empty key, duplicate, insert, no auth write, no active change. Invalid name via `normalizeName`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "test: add Go account without activating or writing auth.json"
```

---

### Task 8: Save

**Files:**

- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestSave_renamesActive(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	newName := gofakeit.LetterN(10)
	if err := s.Save(newName); err != nil {
		t.fatal(err)
	}
	entries, err := s.List()
	if err != nil {
		t.fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != newName || !entries[0].Active {
		t.Fatalf("%+v", entries)
	}
}

func TestSave_sameNameIsNoop(t *testing.T) {
	fx := newFixture(t)
	if err := New(fx.Dir).Save(fx.GoName); err != nil {
		t.fatal(err)
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
		t.fatal(err)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestSave_' -count=1`

Expected: FAIL `s.Save undefined`

- [ ] **Step 3: Write minimal implementation**

```go
func (s *Store) Save(name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	f, err := s.load()
	if err != nil {
		return err
	}
	activeID, ok := f.Active[serviceGo]
	if !ok || activeID == "" {
		return fmt.Errorf("não há conta OpenCode Go ativa para salvar. Faça login ou ocgs add … e ocgs switch.")
	}
	acc, ok := f.Accounts[activeID]
	if !ok || acc.ServiceID != serviceGo {
		return fmt.Errorf("não há conta OpenCode Go ativa para salvar. Faça login ou ocgs add … e ocgs switch.")
	}
	if acc.Description == name {
		return nil
	}
	if matches := s.goByName(f, name); len(matches) > 0 {
		return fmt.Errorf("já existe conta Go chamada %q. Use outro nome ou remova a atual.", name)
	}
	acc.Description = name
	f.Accounts[activeID] = acc
	return writeAtomic(s.accountPath(), f)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: save active OpenCode Go account name"
```

---

### Task 9: Remove

**Files:**

- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestRemove_deletesInactive(t *testing.T) {
	fx := newFixture(t)
	s := New(fx.Dir)
	extra := gofakeit.LetterN(10)
	if err := s.Add(extra, "sk-"+gofakeit.LetterN(40)); err != nil {
		t.fatal(err)
	}
	if err := s.Remove(extra); err != nil {
		t.fatal(err)
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
		t.fatal(err)
	}
	return e
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestRemove_' -count=1`

Expected: FAIL `s.Remove undefined`

- [ ] **Step 3: Write minimal implementation**

```go
func (s *Store) Remove(name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	f, err := s.load()
	if err != nil {
		return err
	}
	found := s.goByName(f, name)
	if len(found) == 0 {
		return s.notFound(name, f)
	}
	if len(found) > 1 {
		return fmt.Errorf("há %d contas Go chamadas %q. Renomeie a ativa com ocgs save <nome-único>.", len(found), name)
	}
	acc := found[0]
	if f.Active[serviceGo] == acc.ID {
		return fmt.Errorf("%q está ativa. Troque com ocgs switch <outra> antes de remover.", name)
	}
	delete(f.Accounts, acc.ID)
	return writeAtomic(s.accountPath(), f)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: remove inactive OpenCode Go accounts"
```

---

### Task 10: ActiveGoKey + file mode 0600 + ID length

**Files:**

- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestActiveGoKey_returnsKey(t *testing.T) {
	fx := newFixture(t)
	key, err := New(fx.Dir).ActiveGoKey()
	if err != nil {
		t.fatal(err)
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
		t.fatal(err)
	}
	info, err := os.Stat(filepath.Join(fx.Dir, "account.json"))
	if err != nil {
		t.fatal(err)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestActiveGoKey_|TestAdd_writes0600AndULID' -count=1`

Expected: FAIL `ActiveGoKey undefined`

- [ ] **Step 3: Write minimal implementation**

```go
type goCredential struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

func (s *Store) ActiveGoKey() (string, error) {
	f, err := s.load()
	if err != nil {
		return "", err
	}
	id := f.Active[serviceGo]
	acc, ok := f.Accounts[id]
	if !ok || acc.ServiceID != serviceGo {
		return "", fmt.Errorf("não há chave Go ativa para verificar.")
	}
	var cred goCredential
	if err := json.Unmarshal(acc.Credential, &cred); err != nil {
		return "", err
	}
	if cred.Key == "" {
		return "", fmt.Errorf("não há chave Go ativa para verificar.")
	}
	return cred.Key, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -count=1`

Expected: PASS (`0600` may fail on Windows; this project targets macOS/darwin as in the spec.)

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: read active Go key and persist files at 0600"
```

---

### Task 11: Verify HTTP client

**Files:**

- Create: `internal/verify/verify.go`
- Create: `internal/verify/verify_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package verify

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerify_accepts2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("auth %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if err := c.Verify("sk-test"); err != nil {
		t.fatal(err)
	}
}

func TestVerify_rejected401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Verify("sk-bad")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "a chave ativa foi recusada pela API OpenCode Go (HTTP 401). Troque de conta ou gere outra chave."
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestVerify_otherHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := (&Client{BaseURL: srv.URL, HTTPClient: srv.Client()}).Verify("sk-x")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "a API OpenCode Go respondeu HTTP 500 ao verificar a chave ativa."
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestVerify_network(t *testing.T) {
	c := &Client{
		BaseURL:    "http://127.0.0.1:1",
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	}
	err := c.Verify("sk-x")
	if err == nil {
		t.Fatal("expected error")
	}
	prefix := "não consegui falar com a API OpenCode Go em "
	if len(err.Error()) < len(prefix) || err.Error()[:len(prefix)] != prefix {
		t.Fatalf("got %q", err.Error())
	}
}
```

Do not import `errors`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/verify/ -count=1`

Expected: FAIL `undefined: Client`

- [ ] **Step 3: Write minimal implementation**

```go
package verify

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://opencode.ai/zen/go/v1"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *Client) Verify(key string) error {
	base := strings.TrimRight(c.BaseURL, "/")
	url := base + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("não consegui falar com a API OpenCode Go em %s: %v. O switch local não depende disso.", url, err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("não consegui falar com a API OpenCode Go em %s: %v. O switch local não depende disso.", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("a chave ativa foi recusada pela API OpenCode Go (HTTP %d). Troque de conta ou gere outra chave.", resp.StatusCode)
	}
	return fmt.Errorf("a API OpenCode Go respondeu HTTP %d ao verificar a chave ativa.", resp.StatusCode)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/verify/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/verify/verify.go internal/verify/verify_test.go
git commit -m "feat: verify OpenCode Go API key over HTTP"
```

---

### Task 12: CLI — list, switch subcommand, positional

**Files:**

- Create: `internal/cli/cli.go`
- Create: `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package cli

import (
	"bytes"
	"encoding/json"
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
	cmd.SetArgs(normalizeArgs(args))
	err = cmd.Execute()
	return out.String(), errBuf.String(), err
}

func TestCLI_listMarksActiveOmitsKey(t *testing.T) {
	dir, goName, goKey := writeCLIFixture(t)
	stdout, _, err := runCLI(t, dir)
	if err != nil {
		t.fatal(err)
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
		t.fatal(err)
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
		t.fatal(err)
	}
	if _, _, err := runCLI(t, dir, "switch", other); err != nil {
		t.fatal(err)
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
		t.fatal(err)
	}
	if _, _, err := runCLI(t, dir, third); err != nil {
		t.fatal(err)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -count=1`

Expected: FAIL `undefined: New` / `undefined: normalizeArgs`

- [ ] **Step 3: Write minimal implementation**

```go
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"agent-account-switchers/internal/store"
)

type Verifier interface {
	Verify(key string) error
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	if len(args[0]) > 0 && args[0][0] == '-' {
		return args
	}
	switch args[0] {
	case "list", "switch", "add", "save", "remove", "verify", "help", "completion":
		return args
	default:
		return append([]string{"switch"}, args...)
	}
}

func New(s *store.Store, v Verifier, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "ocgs",
		Short:         "Troca contas OpenCode Go",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(s, stdout)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Lista contas OpenCode Go",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(s, stdout)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "switch [nome]",
		Short: "Ativa uma conta OpenCode Go",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("informe a conta: ocgs switch <nome>")
			}
			return s.Switch(args[0])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "add <nome> <chave>",
		Short: "Adiciona uma conta OpenCode Go",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("informe a chave: ocgs add <nome> <chave>")
			}
			return s.Add(args[0], args[1])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "save <nome>",
		Short: "Salva a conta Go ativa com um nome",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("informe a conta: ocgs switch <nome>")
			}
			return s.Save(args[0])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "remove <nome>",
		Short: "Remove uma conta OpenCode Go inativa",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("informe a conta: ocgs switch <nome>")
			}
			return s.Remove(args[0])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verifica a chave Go ativa na API",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := s.ActiveGoKey()
			if err != nil {
				return err
			}
			if err := v.Verify(key); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "chave OpenCode Go ativa é válida.")
			return err
		},
	})
	return root
}

func runList(s *store.Store, stdout io.Writer) error {
	entries, err := s.List()
	if err != nil {
		return err
	}
	for _, e := range entries {
		prefix := "  "
		if e.Active {
			prefix = "→ "
		}
		if _, err := fmt.Fprintf(stdout, "%s%s\n", prefix, e.Name); err != nil {
			return err
		}
	}
	return nil
}
```

**Fix save/remove usage errors** before tests that need them: `save`/`remove` without args are not in Task 12 tests. Leave the placeholder message for now; Task 13 will add dedicated messages if we want. Spec only mandates `ocgs switch` empty message. save/remove without args can reuse a generic `informe a conta: ocgs save <nome>` — change save/remove in this same implementation to:

```go
return fmt.Errorf("informe a conta: ocgs save <nome>")
```

and

```go
return fmt.Errorf("informe a conta: ocgs remove <nome>")
```

Do that in Step 3 (not the switch message).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/cli/ -count=1`

Expected: PASS for Task 12 tests. `runCLI` calls `normalizeArgs` **and** `SetArgs`. `New(...).Execute()` with already-normalized args: `runCLI(t, dir, third)` → `normalizeArgs` → `["switch", third]`. `runCLI(t, dir)` → empty args → root `RunE` lists. Good.

**Do not double-normalize in `Execute`.** `main` will call `normalizeArgs(os.Args[1:])` then `SetArgs`. Tests call `normalizeArgs` in `runCLI`. Keep `New` unaware of os.Args.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: CLI list and switch (subcommand and positional)"
```

---

### Task 13: CLI add, save, remove, verify, missing store

**Files:**

- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestCLI_addSaveRemove(t *testing.T) {
	dir, goName, _ := writeCLIFixture(t)
	s := store.New(dir)
	added := gofakeit.LetterN(10)
	key := "sk-" + gofakeit.LetterN(40)
	if _, _, err := runCLI(t, dir, "add", added, key); err != nil {
		t.fatal(err)
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
		t.fatal(err)
	}
	if _, _, err := runCLI(t, dir, "remove", added); err != nil {
		t.fatal(err)
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
		t.fatal(err)
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
```

Add `"fmt"` to `cli_test.go` imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run 'TestCLI_addSaveRemove|TestCLI_verifyOKAnd401|TestCLI_missingStore' -count=1`

Expected: FAIL until add/save/remove/verify commands exist. If Task 12 already added them, tests may PASS immediately — then commit tests only.

- [ ] **Step 3: Implementation**

Already present from Task 12 if Step 3 there added add/save/remove/verify. If `TestCLI_addSaveRemove` fails because `save` usage error is wrong, fix `save` RunE. No other production code.

- [ ] **Step 4: Run tests**

Run: `go test ./... -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: CLI add, save, remove, verify"
```

---

### Task 14: `main` + README

**Files:**

- Create: `cmd/ocgs/main.go`
- Create: `README.md`

`main` is a wiring file. Cover path wiring with a small test in `internal/store` (already done). Optional smoke: `go build ./cmd/ocgs`.

- [ ] **Step 1: Write main**

```go
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"agent-account-switchers/internal/cli"
	"agent-account-switchers/internal/store"
	"agent-account-switchers/internal/verify"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := store.ResolveDataDir(os.Getenv, home)
	s := store.New(dir)
	v := &verify.Client{
		BaseURL:    verify.DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	cmd := cli.New(s, v, os.Stdout, os.Stderr)
	cmd.SetArgs(cli.NormalizeArgs(os.Args[1:]))
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

`normalizeArgs` is currently unexported. **Export it** as `NormalizeArgs` in `internal/cli/cli.go` and update `cli_test.go` + `main.go`.

- [ ] **Step 2: Export NormalizeArgs and fix tests**

Rename `normalizeArgs` → `NormalizeArgs` in `cli.go` and `cli_test.go`.

- [ ] **Step 3: Build and test**

Run:

```bash
go test ./... -count=1
go build -o ocgs ./cmd/ocgs
```

Expected: PASS, binary produced.

- [ ] **Step 4: README.md**

````markdown
# ocgs

CLI em Go para trocar contas **OpenCode Go** no store nativo do OpenCode 1.18 (`~/.local/share/opencode/account.json` + `auth.json`).

Outros provedores (Copilot, OpenAI, Zen, …) não são alterados. Sessões OpenCode já abertas continuam com a conta antiga; o switch vale no próximo launch.

## Install

```bash
go install agent-account-switchers/cmd/ocgs@latest
```
````

Ou neste repo:

```bash
go build -o ocgs ./cmd/ocgs
```

Override do diretório: `OPENCODE_DATA_DIR=/caminho ocgs list`.

## Comandos

```
ocgs                     # lista contas Go (→ na ativa)
ocgs list
ocgs trabalho            # troca para "trabalho"
ocgs switch trabalho
ocgs add trabalho sk-…   # cria, não ativa
ocgs save trabalho       # nomeia a conta Go ativa
ocgs remove trabalho     # apaga conta inativa
ocgs verify              # GET na API Go com a chave ativa
```

Arquivos de credencial são gravados com modo `0600`. A CLI nunca imprime a API key.

````

- [ ] **Step 5: Commit**

```bash
git add cmd/ocgs/main.go internal/cli/cli.go internal/cli/cli_test.go README.md
git commit -m "feat: wire ocgs main and document usage"
````

- [ ] **Step 6: Final gate**

Run: `go test ./... -count=1`

Expected: all packages PASS. Delete the `./ocgs` binary if created (it is gitignored).

---

## Self-review vs spec

| Spec item                                           | Task                     |
| --------------------------------------------------- | ------------------------ |
| Só OpenCode Go                                      | 4–9 (filter `serviceID`) |
| `save` + `add`                                      | 7, 8, 13                 |
| `ocgs`, `list`, `switch`, positional                | 12, 14                   |
| Persist `account.json` + sync `auth.json` on switch | 5                        |
| Preserve other providers                            | 5                        |
| `verify` optional, not on switch                    | 11, 12                   |
| Missing store / invalid JSON                        | 3, 13                    |
| Unknown name, duplicate name                        | 6, 9                     |
| Duplicate add/save                                  | 7, 8                     |
| Empty key, invalid name                             | 7                        |
| Save without active                                 | 8                        |
| Remove active refused                               | 9                        |
| Verify 401 / other HTTP / network / 2xx             | 11                       |
| Write 0600, atomic                                  | 5 (`writeAtomic`), 10    |
| ULID 26 Crockford                                   | 5 `id.go`, 10            |
| Cobra + gofakeit TDD                                | 1, 7, 12                 |
| README + sessões abertas                            | 14                       |
| `OPENCODE_DATA_DIR` / XDG / home                    | 2, 14                    |
| No keys on stdout                                   | 4, 12, 13                |

No remaining spec command without a task.
