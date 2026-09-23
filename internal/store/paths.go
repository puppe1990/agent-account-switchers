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
