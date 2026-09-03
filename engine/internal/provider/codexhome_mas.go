//go:build mas

package provider

// resolveCodexHome has no fallback in the App Store build. The sandbox cannot
// read ~/.codex — the path resolves inside the app container, where the Codex
// CLI never wrote anything — so pretending to look there would only produce a
// confusing "logged in" that tars could not use.
func resolveCodexHome(linettaCodexHome string) string { return linettaCodexHome }
