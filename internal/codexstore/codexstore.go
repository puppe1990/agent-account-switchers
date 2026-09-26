// Package codexstore is a minimal, read-oriented view over the codex-auth
// account registry (github.com/Loongphy/codex-auth). It reads the accounts
// saved under <home>/accounts/registry.json and switches the active session by
// copying the account auth file over <home>/auth.json, enough for the web UI to
// list and switch accounts.
package codexstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Name   string
	Active bool
}

type Store struct {
	Dir string
	Now func() time.Time
}

func New(dir string) *Store { return &Store{Dir: dir, Now: time.Now} }

func ResolveDataDir(getenv func(string) string, home string) string {
	if dir := getenv("CODEX_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".codex")
}

func (s *Store) registryPath() string { return filepath.Join(s.Dir, "accounts", "registry.json") }
func (s *Store) authPath() string     { return filepath.Join(s.Dir, "auth.json") }

func (s *Store) accountAuthPath(key string) string {
	name := base64.RawURLEncoding.EncodeToString([]byte(key)) + ".auth.json"
	return filepath.Join(s.Dir, "accounts", name)
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type account struct {
	fields map[string]any
	key    string
	name   string
}

func loadAccounts(reg map[string]any) []account {
	raw, _ := reg["accounts"].([]any)
	accounts := make([]account, 0, len(raw))
	for _, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := fields["account_key"].(string)
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		accounts = append(accounts, account{fields: fields, key: key, name: accountName(fields)})
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].name != accounts[j].name {
			return accounts[i].name < accounts[j].name
		}
		return accounts[i].key < accounts[j].key
	})
	return accounts
}

func accountName(fields map[string]any) string {
	for _, field := range []string{"alias", "account_name", "email"} {
		if value, ok := fields[field].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	key, _ := fields["account_key"].(string)
	return strings.TrimSpace(key)
}

func (a account) matches(ref string) bool {
	if strings.EqualFold(a.key, ref) {
		return true
	}
	for _, field := range []string{"alias", "account_name", "email"} {
		if value, ok := a.fields[field].(string); ok && strings.EqualFold(strings.TrimSpace(value), ref) {
			return true
		}
	}
	return false
}

func notFound(name string, accounts []account) error {
	if len(accounts) == 0 {
		return fmt.Errorf("conta %q não existe. não há contas Codex", name)
	}
	names := make([]string, 0, len(accounts))
	for _, a := range accounts {
		names = append(names, a.name)
	}
	return fmt.Errorf("conta %q não existe. Contas Codex: %s.", name, strings.Join(names, ", "))
}

func (s *Store) loadRegistry() (map[string]any, error) {
	data, err := os.ReadFile(s.registryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var reg map[string]any
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("registry.json em %s não é um JSON válido: %w", s.registryPath(), err)
	}
	return reg, nil
}

func (s *Store) saveRegistry(reg map[string]any) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.registryPath(), data)
}

func (s *Store) List() ([]Entry, error) {
	reg, err := s.loadRegistry()
	if err != nil {
		return nil, err
	}
	active, _ := reg["active_account_key"].(string)
	accounts := loadAccounts(reg)
	entries := make([]Entry, 0, len(accounts))
	for _, a := range accounts {
		entries = append(entries, Entry{Name: a.name, Active: active != "" && a.key == active})
	}
	return entries, nil
}

func (s *Store) Switch(name string) error {
	ref := strings.TrimSpace(name)
	if ref == "" {
		return fmt.Errorf("informe a conta")
	}
	reg, err := s.loadRegistry()
	if err != nil {
		return err
	}
	accounts := loadAccounts(reg)
	var target *account
	for i := range accounts {
		if accounts[i].matches(ref) {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return notFound(name, accounts)
	}

	s.syncActiveAuth(accounts)

	path := s.accountAuthPath(target.key)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("não achei a credencial da conta %q em %s", target.name, path)
		}
		return err
	}
	if !json.Valid(data) {
		return fmt.Errorf("credencial da conta %q em %s não é um JSON válido", target.name, path)
	}
	if err := writeAtomic(s.authPath(), data); err != nil {
		return err
	}
	now := s.now()
	reg["active_account_key"] = target.key
	reg["active_account_activated_at_ms"] = now.UnixMilli()
	target.fields["last_used_at"] = now.Unix()
	return s.saveRegistry(reg)
}

type authFile struct {
	APIKey string `json:"OPENAI_API_KEY"`
	Tokens struct {
		AccountID string `json:"account_id"`
	} `json:"tokens"`
}

// Sync stores the live auth.json in the saved account it belongs to, so tokens
// refreshed by Codex are not lost before a switch or a new login.
func (s *Store) Sync() error {
	reg, err := s.loadRegistry()
	if err != nil {
		return err
	}
	s.syncActiveAuth(loadAccounts(reg))
	return nil
}

// ImportCurrent saves the account in the live auth.json into the registry, the
// same way `codex-auth login` does once a sign in completes.
func (s *Store) ImportCurrent() (string, error) {
	data, err := os.ReadFile(s.authPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("não há auth do Codex em %s", s.authPath())
		}
		return "", err
	}
	var auth struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			IDToken   string `json:"id_token"`
			AccountID string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return "", fmt.Errorf("auth.json em %s não é um JSON válido: %w", s.authPath(), err)
	}
	accountID := strings.TrimSpace(auth.Tokens.AccountID)
	if accountID == "" {
		return "", fmt.Errorf("a auth atual não é de uma conta ChatGPT")
	}
	claims, err := parseIDToken(auth.Tokens.IDToken)
	if err != nil {
		return "", err
	}
	if claims.Auth.AccountID != "" && claims.Auth.AccountID != accountID {
		return "", fmt.Errorf("id_token e account_id da auth atual não batem")
	}
	userID := claims.Auth.UserID
	if userID == "" {
		userID = claims.Auth.LegacyUserID
	}
	if userID == "" {
		return "", fmt.Errorf("id_token sem chatgpt_user_id")
	}

	key := userID + "::" + accountID
	if err := writeAtomic(s.accountAuthPath(key), data); err != nil {
		return "", err
	}

	reg, err := s.loadRegistry()
	if err != nil {
		return "", err
	}
	now := s.now()
	record := recordFor(reg, key)
	if record == nil {
		record = map[string]any{
			"account_key":        key,
			"chatgpt_account_id": accountID,
			"chatgpt_user_id":    userID,
			"alias":              "",
			"account_name":       nil,
			"created_at":         now.Unix(),
		}
		raw, _ := reg["accounts"].([]any)
		reg["accounts"] = append(raw, record)
		if _, ok := reg["schema_version"]; !ok {
			reg["schema_version"] = 3
		}
	}
	if email := strings.TrimSpace(claims.Email); email != "" {
		record["email"] = email
	}
	if plan := strings.TrimSpace(claims.Auth.Plan); plan != "" {
		record["plan"] = plan
	}
	if auth.AuthMode != "" {
		record["auth_mode"] = auth.AuthMode
	}
	record["last_used_at"] = now.Unix()
	reg["active_account_key"] = key
	reg["active_account_activated_at_ms"] = now.UnixMilli()
	if err := s.saveRegistry(reg); err != nil {
		return "", err
	}
	return accountName(record), nil
}

func recordFor(reg map[string]any, key string) map[string]any {
	raw, _ := reg["accounts"].([]any)
	for _, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if value, _ := fields["account_key"].(string); strings.TrimSpace(value) == key {
			return fields
		}
	}
	return nil
}

type idTokenClaims struct {
	Email string `json:"email"`
	Auth  struct {
		AccountID    string `json:"chatgpt_account_id"`
		UserID       string `json:"chatgpt_user_id"`
		LegacyUserID string `json:"user_id"`
		Plan         string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

func parseIDToken(token string) (idTokenClaims, error) {
	var claims idTokenClaims
	token = strings.TrimSpace(token)
	if token == "" {
		return claims, fmt.Errorf("a auth atual não tem id_token")
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return claims, fmt.Errorf("id_token da auth atual não é um JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, fmt.Errorf("id_token da auth atual não é um JWT: %w", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, fmt.Errorf("claims do id_token inválidos: %w", err)
	}
	return claims, nil
}

// syncActiveAuth copies the live auth.json back into the saved account it
// belongs to, so tokens refreshed by Codex since the last switch are not lost.
func (s *Store) syncActiveAuth(accounts []account) {
	live, err := os.ReadFile(s.authPath())
	if err != nil {
		return
	}
	var current authFile
	if err := json.Unmarshal(live, &current); err != nil {
		return
	}
	for _, a := range accounts {
		path := s.accountAuthPath(a.key)
		stored, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var saved authFile
		if err := json.Unmarshal(stored, &saved); err != nil {
			continue
		}
		if sameAccount(current, saved) {
			_ = writeAtomic(path, live)
			return
		}
	}
}

func sameAccount(live, stored authFile) bool {
	if live.Tokens.AccountID != "" {
		return stored.Tokens.AccountID == live.Tokens.AccountID
	}
	return live.APIKey != "" && stored.APIKey == live.APIKey
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
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
