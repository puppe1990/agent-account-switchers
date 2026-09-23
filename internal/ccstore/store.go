package ccstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Auth struct {
	APIKey          string `json:"apiKey"`
	UserID          string `json:"userId,omitempty"`
	UserName        string `json:"userName,omitempty"`
	KeyName         string `json:"keyName,omitempty"`
	AuthenticatedAt string `json:"authenticatedAt,omitempty"`
}

type Ledger struct {
	Version  int             `json:"version"`
	Accounts map[string]Auth `json:"accounts"`
	Active   string          `json:"active"`
}

type Entry struct {
	Name   string
	Active bool
}

type Store struct {
	Dir string
	Now func() time.Time
}

func New(dir string) *Store {
	return &Store{Dir: dir, Now: time.Now}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) authPath() string   { return filepath.Join(s.Dir, "auth.json") }
func (s *Store) ledgerPath() string { return filepath.Join(s.Dir, "ccs-accounts.json") }

func (s *Store) missingAuth() error {
	return fmt.Errorf("não achei o auth do Command Code em %s. Rode cmd login.", s.Dir)
}

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

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("nome de conta inválido: %q. Use um nome sem \"/\" e não vazio.", name)
	}
	return name, nil
}

func (s *Store) loadAuth() (Auth, error) {
	b, err := os.ReadFile(s.authPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Auth{}, s.missingAuth()
		}
		return Auth{}, err
	}
	var a Auth
	if err := json.Unmarshal(b, &a); err != nil {
		return Auth{}, fmt.Errorf("auth.json em %s não é um JSON válido: %v", s.authPath(), err)
	}
	return a, nil
}

func (s *Store) loadLedger() (*Ledger, error) {
	b, err := os.ReadFile(s.ledgerPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Ledger{Version: 1, Accounts: map[string]Auth{}}, nil
		}
		return nil, err
	}
	var l Ledger
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("ccs-accounts.json em %s não é um JSON válido: %v", s.ledgerPath(), err)
	}
	if l.Accounts == nil {
		l.Accounts = map[string]Auth{}
	}
	if l.Version == 0 {
		l.Version = 1
	}
	return &l, nil
}

func (s *Store) names(l *Ledger) []string {
	names := make([]string, 0, len(l.Accounts))
	for n := range l.Accounts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (s *Store) notFound(name string, l *Ledger) error {
	names := s.names(l)
	if len(names) == 0 {
		return fmt.Errorf("conta %q não existe. não há contas Command Code", name)
	}
	return fmt.Errorf("conta %q não existe. Contas Command Code: %s.", name, strings.Join(names, ", "))
}

func (s *Store) currentKey() string {
	a, err := s.loadAuth()
	if err != nil {
		return ""
	}
	return a.APIKey
}

func (s *Store) List() ([]Entry, error) {
	l, err := s.loadLedger()
	if err != nil {
		return nil, err
	}
	cur := s.currentKey()
	var entries []Entry
	for _, name := range s.names(l) {
		acc := l.Accounts[name]
		entries = append(entries, Entry{Name: name, Active: cur != "" && acc.APIKey == cur})
	}
	return entries, nil
}

func (s *Store) Add(name, key string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("informe a chave: ccs add <nome> <chave>")
	}
	l, err := s.loadLedger()
	if err != nil {
		return err
	}
	if _, ok := l.Accounts[name]; ok {
		return fmt.Errorf("já existe conta Command Code chamada %q. Use outro nome ou remova a atual.", name)
	}
	l.Accounts[name] = Auth{
		APIKey:          key,
		UserName:        name,
		KeyName:         "cli-manual-entry",
		AuthenticatedAt: s.now().UTC().Format(time.RFC3339),
	}
	return writeAtomic(s.ledgerPath(), l)
}

func (s *Store) Switch(name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	l, err := s.loadLedger()
	if err != nil {
		return err
	}
	acc, ok := l.Accounts[name]
	if !ok {
		return s.notFound(name, l)
	}
	l.Active = name
	if err := writeAtomic(s.ledgerPath(), l); err != nil {
		return err
	}
	return writeAtomic(s.authPath(), acc)
}

func (s *Store) Save(name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	auth, err := s.loadAuth()
	if err != nil {
		return err
	}
	if auth.APIKey == "" {
		return fmt.Errorf("não há conta Command Code ativa para salvar. Faça login com cmd login ou ccs add … e ccs switch.")
	}
	l, err := s.loadLedger()
	if err != nil {
		return err
	}
	if existing, ok := l.Accounts[name]; ok && existing.APIKey != auth.APIKey {
		return fmt.Errorf("já existe conta Command Code chamada %q. Use outro nome ou remova a atual.", name)
	}
	l.Accounts[name] = auth
	l.Active = name
	return writeAtomic(s.ledgerPath(), l)
}

func (s *Store) Remove(name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	l, err := s.loadLedger()
	if err != nil {
		return err
	}
	acc, ok := l.Accounts[name]
	if !ok {
		return s.notFound(name, l)
	}
	if cur := s.currentKey(); cur != "" && acc.APIKey == cur {
		return fmt.Errorf("%q está ativa. Troque com ccs switch <outra> antes de remover.", name)
	}
	delete(l.Accounts, name)
	if l.Active == name {
		l.Active = ""
	}
	return writeAtomic(s.ledgerPath(), l)
}

func (s *Store) ActiveKey() (string, error) {
	auth, err := s.loadAuth()
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("não há chave Command Code ativa para verificar.")
		}
		if err.Error() == s.missingAuth().Error() {
			return "", fmt.Errorf("não há chave Command Code ativa para verificar.")
		}
		return "", err
	}
	if auth.APIKey == "" {
		return "", fmt.Errorf("não há chave Command Code ativa para verificar.")
	}
	return auth.APIKey, nil
}
