package codexlogin

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenPrivate opens url in a private/incognito window of the first Chromium
// browser found, falling back to the default browser.
func OpenPrivate(url string) error {
	switch runtime.GOOS {
	case "darwin":
		for _, app := range []string{"Google Chrome", "Brave Browser", "Microsoft Edge", "Chromium"} {
			if err := exec.Command("open", "-na", app, "--args", "--incognito", url).Run(); err == nil {
				return nil
			}
		}
		return exec.Command("open", url).Run()
	case "windows":
		for _, exe := range []string{"chrome", "msedge", "brave", "chromium"} {
			if err := exec.Command(exe, "--incognito", url).Start(); err == nil {
				return nil
			}
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		for _, exe := range []string{"google-chrome", "brave-browser", "chromium", "chromium-browser", "microsoft-edge"} {
			if err := exec.Command(exe, "--incognito", url).Start(); err == nil {
				return nil
			}
		}
		return exec.Command("xdg-open", url).Start()
	}
}

// shim hides the browser from `codex login`: the CLI calls the webbrowser
// crate, so a no-op `open`/`xdg-open` earlier in PATH keeps the default
// browser (already logged in) out of the flow.
func (m *Manager) shim() []string {
	m.shimOnce.Do(func() {
		if runtime.GOOS == "windows" {
			return
		}
		dir, err := os.MkdirTemp("", "switcher-ui-nobrowser-")
		if err != nil {
			return
		}
		script := "#!/bin/sh\nexit 0\n"
		for _, name := range []string{"open", "xdg-open"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
				return
			}
		}
		path := dir + string(os.PathListSeparator) + os.Getenv("PATH")
		m.shimEnv = []string{"PATH=" + path, "BROWSER=" + filepath.Join(dir, "open")}
	})
	return m.shimEnv
}
