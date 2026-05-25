package ai

import (
	"github.com/devlikebear/tars/pkg/llm"
)

// ClientFactory creates an llm.Client for a given provider name. Wraps
// llm.NewProvider so tests can inject a fake without monkey-patching tars.
type ClientFactory func(provider, workDir string) (llm.Client, error)

// DefaultClientFactory delegates to tars.
func DefaultClientFactory(provider, workDir string) (llm.Client, error) {
	return llm.NewProvider(llm.ProviderOptions{
		Provider: provider,
		WorkDir:  workDir,
	})
}
