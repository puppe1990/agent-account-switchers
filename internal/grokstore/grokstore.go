// Package grokstore is a minimal, read-oriented view over the Grok Build CLI
// account switcher (github.com/puppe1990/grok-accounts). It reads the saved
// profiles under <home>/accounts and switches the active session by rewriting
// <home>/auth.json, enough for the web UI to list and switch accounts.
package grokstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Name    string
	Active  bool
	Warning string
}

type Store struct {
	Dir string
	mu  sync.Mutex
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
	path  string
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
	s.mu.Lock()
	defer s.mu.Unlock()
	profiles, err := s.profiles()
	if err != nil {
		return nil, err
	}
	raw, err := s.readLive()
	if err != nil {
		return nil, err
	}
	live := parseEmail(raw)
	if err := snapshotActive(profiles, live, raw); err != nil {
		return nil, fmt.Errorf("não foi possível guardar a sessão renovada do Grok: %w", err)
	}
	entries := make([]Entry, 0, len(profiles))
	for _, p := range profiles {
		auth := []byte(p.Auth)
		if live != "" && strings.EqualFold(live, strings.TrimSpace(p.Email)) {
			auth = raw
		}
		entries = append(entries, Entry{
			Name:    p.name(),
			Warning: credentialWarning(auth),
			Active:  live != "" && strings.EqualFold(live, strings.TrimSpace(p.Email)),
		})
	}
	return entries, nil
}

func (s *Store) Switch(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	if len(found.Auth) == 0 || bytes.Equal(bytes.TrimSpace(found.Auth), []byte("null")) {
		return fmt.Errorf("perfil %q não tem credencial salva", found.name())
	}
	live, err := os.ReadFile(s.authPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	email := parseEmail(live)
	if email != "" && strings.EqualFold(email, strings.TrimSpace(found.Email)) {
		return nil
	}
	if err := snapshotActive(profiles, email, live); err != nil {
		return err
	}
	if identity := parseEmail(found.Auth); identity == "" || !strings.EqualFold(identity, strings.TrimSpace(found.Email)) {
		return fmt.Errorf("não foi possível ativar %q: a credencial não corresponde à conta salva. Faça login com grok-accounts login para atualizar o perfil", found.name())
	}
	if warning := credentialWarning(found.Auth); warning == missingRefreshWarning {
		return fmt.Errorf("não foi possível ativar %q: %s", found.name(), warning)
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
		p.path = filepath.Join(dir, fileName)
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].name() < profiles[j].name() })
	return profiles, nil
}

func (s *Store) readLive() ([]byte, error) {
	data, err := os.ReadFile(s.authPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
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

// Preserve refreshed credentials before restoring another account's snapshot.
func snapshotActive(profiles []profile, email string, auth []byte) error {
	for _, p := range profiles {
		if email == "" || !strings.EqualFold(strings.TrimSpace(p.Email), email) {
			continue
		}
		return updateSnapshot(p.path, auth)
	}
	return nil
}

func updateSnapshot(path string, auth []byte) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if equalJSON(fields["auth"], auth) {
		return nil
	}
	fields["auth"] = json.RawMessage(auth)
	fields["saved_at"], err = json.Marshal(time.Now().UTC())
	if err != nil {
		return err
	}
	updated, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, updated)
}

const missingRefreshWarning = "O token expirou e não pode ser renovado. Faça login com grok-accounts login para atualizar o perfil."

func credentialWarning(raw []byte) string {
	var entries map[string]struct {
		Email        string `json:"email"`
		ExpiresAt    string `json:"expires_at"`
		RefreshToken string `json:"refresh_token"`
	}
	if json.Unmarshal(raw, &entries) != nil {
		return "Credencial ilegível. Faça login novamente com grok-accounts login."
	}
	for _, entry := range entries {
		expiry, err := time.Parse(time.RFC3339Nano, entry.ExpiresAt)
		if entry.Email == "" || err != nil || expiry.After(time.Now()) {
			continue
		}
		if entry.RefreshToken == "" {
			return missingRefreshWarning
		}
		return "Token vencido; o Grok precisa renová-lo. Se pedir login, use grok-accounts login."
	}
	return ""
}

func equalJSON(a, b []byte) bool {
	var compactA, compactB bytes.Buffer
	if json.Compact(&compactA, a) != nil || json.Compact(&compactB, b) != nil {
		return false
	}
	return bytes.Equal(compactA.Bytes(), compactB.Bytes())
}
