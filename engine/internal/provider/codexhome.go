//go:build !mas

package provider

import (
	"os"
	"path/filepath"
)

// resolveCodexHome decides which directory tars should read Codex's auth.json
// from. Linetta's own login always wins; a Codex CLI login is honoured only
// when Linetta has none, so somebody who already ran `codex login` does not
// have to log in twice.
//
// Writes never come here — the login (#92) always stores into Linetta's own
// directory. This is a read-side convenience, and it is re-evaluated on every
// Resolve so a login performed mid-session takes effect immediately.
func resolveCodexHome(linettaCodexHome string) string {
	if linettaCodexHome == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(linettaCodexHome, codexAuthFile)); err == nil {
		return linettaCodexHome
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return linettaCodexHome
	}
	cli := filepath.Join(home, ".codex")
	if _, err := os.Stat(filepath.Join(cli, codexAuthFile)); err == nil {
		return cli
	}
	return linettaCodexHome
}
