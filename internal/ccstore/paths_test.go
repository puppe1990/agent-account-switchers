package ccstore

import "testing"

func TestResolveDataDir_ccsDataDirWins(t *testing.T) {
	got := ResolveDataDir(func(k string) string {
		if k == "CCS_DATA_DIR" {
			return "/tmp/custom-ccs"
		}
		return ""
	}, "/home/dev")
	if got != "/tmp/custom-ccs" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveDataDir_homeFallback(t *testing.T) {
	got := ResolveDataDir(func(string) string { return "" }, "/home/dev")
	if got != "/home/dev/.commandcode" {
		t.Fatalf("got %q", got)
	}
}
