// Package grokstore is a minimal, read-oriented view over the Grok Build CLI
// account switcher (github.com/puppe1990/grok-accounts). It reads the saved
// profiles under <home>/accounts and switches the active session by rewriting
// <home>/auth.json, enough for the web UI to list and switch accounts.
package grokstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Entry struct {
	Name   string
	Active bool
}

type Store struct {
	Dir string
}

func New(dir string) *Store { return &Store{Dir: dir} }

func ResolveDataDir(getenv func(string) string, home string) string {
	if dir := getenv("GROK_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".grok")
}

func (s *Store) accountsDir() string { return filepath.Join(s.Dir, "accounts") }
func (s *Store) authPath() string    { return filepath.Join(s.Dir, "auth.json") }

type profile struct {
	Alias string          `json:"alias"`
	Email string          `json:"email"`
	Auth  json.RawMessage `json:"auth"`
}

func (p profile) name() string {
	if alias := strings.TrimSpace(p.Alias); alias != "" {
		return alias
	}
	if email := strings.TrimSpace(p.Email); email != "" {
		return email
	}
	return "perfil"
}

func (s *Store) List() ([]Entry, error) {
	profiles, err := s.profiles()
	if err != nil {
		return nil, err
	}
	live, err := s.liveEmail()
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(profiles))
	for _, p := range profiles {
		entries = append(entries, Entry{
			Name:   p.name(),
			Active: live != "" && strings.EqualFold(live, strings.TrimSpace(p.Email)),
		})
	}
	return entries, nil
}

func (s *Store) Switch(name string) error {
	profiles, err := s.profiles()
	if err != nil {
		return err
	}
	ref := strings.TrimSpace(name)
	var found *profile
	for i := range profiles {
		p := profiles[i]
		if strings.EqualFold(p.Alias, ref) || strings.EqualFold(p.Email, ref) {
			found = &profiles[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("conta %q não existe", name)
	}
	if len(found.Auth) == 0 {
		return fmt.Errorf("perfil %q não tem credencial salva", found.name())
	}
	return writeAtomic(s.authPath(), found.Auth)
}

func (s *Store) profiles() ([]profile, error) {
	dir := s.accountsDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := make([]profile, 0, len(entries))
	for _, e := range entries {
		fileName := e.Name()
		if e.IsDir() || strings.HasPrefix(fileName, ".") || !strings.HasSuffix(fileName, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, fileName))
		if err != nil {
			return nil, err
		}
		var p profile
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("perfil corrompido %s: %w", fileName, err)
		}
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].name() < profiles[j].name() })
	return profiles, nil
}

func (s *Store) liveEmail() (string, error) {
	data, err := os.ReadFile(s.authPath())
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return parseEmail(data), nil
}

func parseEmail(raw []byte) string {
	var entries map[string]struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return ""
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if email := strings.TrimSpace(entries[key].Email); email != "" {
			return email
		}
	}
	return ""
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".auth.json.tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	writeErr := writeContents(tmp, data)
	if closeErr := tmp.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = os.Rename(tmpName, path)
	}
	if writeErr != nil {
		_ = os.Remove(tmpName)
	}
	return writeErr
}

func writeContents(f *os.File, data []byte) error {
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	_, err := f.Write(data)
	return err
}
