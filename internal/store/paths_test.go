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
