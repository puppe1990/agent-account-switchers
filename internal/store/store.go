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
