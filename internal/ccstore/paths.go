package ccstore

import "path/filepath"

func ResolveDataDir(getenv func(string) string, home string) string {
	if dir := getenv("CCS_DATA_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".commandcode")
}
