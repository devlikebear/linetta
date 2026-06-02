package ai

import (
	"os"
	"testing"
)

func TestDefaultClientFactorySetsClaudeCliPath(t *testing.T) {
	os.Unsetenv("CLAUDE_CODE_CLI_PATH")
	// claude-code-cli with a bogus path: NewProvider may fail to find the binary,
	// but the env var must be set first — that is the behavior under test.
	_, _ = DefaultClientFactory(ResolvedProvider{
		Provider: "claude-code-cli",
		CliPath:  "/tmp/does-not-exist-claude",
	})
	if got := os.Getenv("CLAUDE_CODE_CLI_PATH"); got != "/tmp/does-not-exist-claude" {
		t.Fatalf("CLAUDE_CODE_CLI_PATH=%q, want /tmp/does-not-exist-claude", got)
	}
}
