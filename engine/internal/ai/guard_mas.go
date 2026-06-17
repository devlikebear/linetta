//go:build mas

package ai

import "fmt"

// unavailableMASProviders cannot function in the App Store (sandboxed) build:
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
		return fmt.Errorf("the %q provider is not available in the App Store build", provider)
	}
	return nil
}
