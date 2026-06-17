//go:build mas

package ai

import "fmt"

// guardProvider rejects providers that spawn external processes, which the App
// Store (sandboxed) build cannot do.
func guardProvider(provider string) error {
	if provider == "claude-code-cli" {
		return fmt.Errorf("the %q provider is not available in the App Store build", provider)
	}
	return nil
}
