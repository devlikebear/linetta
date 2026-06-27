//go:build mas || mobile

package ai

import (
	"fmt"
	"sort"
)

// unavailableMASProviders cannot function in the App Store or mobile (sandboxed) builds:
//   - claude-code-cli spawns the `claude` binary (subprocess exec is blocked)
//   - openai-codex reads ~/.codex/auth.json, which under the sandbox resolves to
//     the app container and so cannot reach the user's real Codex credentials
var unavailableMASProviders = map[string]struct{}{
	"claude-code-cli": {},
	"openai-codex":    {},
}

// guardProvider rejects providers that cannot function in the sandboxed build.
func guardProvider(provider string) error {
	if _, blocked := unavailableMASProviders[provider]; blocked {
		return fmt.Errorf("the %q provider is not available in App Store or mobile builds", provider)
	}
	return nil
}

// UnavailableProviders returns the sandboxed build's unusable providers, sorted
// for deterministic output, so the UI can hide them.
func UnavailableProviders() []string {
	out := make([]string, 0, len(unavailableMASProviders))
	for p := range unavailableMASProviders {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
