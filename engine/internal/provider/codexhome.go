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
// It is re-evaluated on every Resolve, so a login performed mid-session takes
// effect immediately.
//
// This is a read-side convenience as far as codexauth is concerned — Linetta's
// own login always writes into Linetta's own directory — but it is NOT
// read-only for the system. tars refreshes an expired token with
// PersistSource: true, rewriting whatever path it resolved from CodexHome. So
// while the fallback is active, a token refresh rewrites the Codex CLI's own
// ~/.codex/auth.json. The privacy policy says so; two loose ends it creates —
// whether Logout should delete a file the CLI owns, and which component wins
// on "is this writer signed in" (codexauth.Status reads only Linetta's home,
// Resolved.Configured stats the resolved one) — are recorded as open
// questions in the #92 plan for the settings pane (#94) to decide.
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
