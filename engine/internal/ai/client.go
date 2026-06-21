package ai

import (
	"os"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// ResolvedProvider is the per-call provider configuration handed to the factory:
// the active provider id plus its model, credential, optional CLI path override,
// and the working directory.
type ResolvedProvider struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	CliPath  string
	WorkDir  string
}

// ClientFactory creates an llm.Client from a resolved provider config. Wraps
// llm.NewProvider so tests can inject a fake without monkey-patching tars.
type ClientFactory func(p ResolvedProvider) (llm.Client, error)

// DefaultClientFactory delegates to tars. For claude-code-cli it injects the
// configured CLI path via the CLAUDE_CODE_CLI_PATH env var that tars reads
// (NewClaudeCodeCLIClient has no path parameter, so the env var is the only hook).
func DefaultClientFactory(p ResolvedProvider) (llm.Client, error) {
	opts := providerOptionsForTars(p)
	if err := guardProvider(opts.Provider); err != nil {
		return nil, err
	}
	if p.Provider == "claude-code-cli" {
		if path := strings.TrimSpace(p.CliPath); path != "" {
			_ = os.Setenv("CLAUDE_CODE_CLI_PATH", path)
		}
	}
	return llm.NewProvider(opts)
}

func providerOptionsForTars(p ResolvedProvider) llm.ProviderOptions {
	provider := p.Provider
	baseURL := p.BaseURL
	if provider == settings.ProviderOpenRouter {
		provider = settings.ProviderOpenAI
		if strings.TrimSpace(baseURL) == "" {
			baseURL = settings.OpenRouterBaseURL
		}
	}
	return llm.ProviderOptions{
		Provider: provider,
		Model:    p.Model,
		APIKey:   p.APIKey,
		BaseURL:  baseURL,
		WorkDir:  p.WorkDir,
	}
}
