package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const serviceGo = "opencode-go"

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
		return nil, fmt.Errorf("account.json em %s não é um JSON v2 válido: %w", s.accountPath(), err)
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

func writeAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("não consegui gravar %s: %w. Nada foi alterado.", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("não consegui gravar %s: %w. Nada foi alterado.", path, err)
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
