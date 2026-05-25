package agent

import (
	"github.com/devlikebear/linetta/internal/llm"
)

// Tests in this package must not shell out to a real codex CLI or hit
// OpenAI. The override below replaces resolveLLMProvider with a stub
// that always returns ErrNoProvider, so runEpisodeCore takes the
// deterministic-buildOutput fallback path (matching the pre-Phase-10
// behavior the existing assertions were written against).
//
// _test.go suffix scopes this to `go test` builds only; production
// binaries link the original runner.go variable.
func init() {
	resolveLLMProvider = func() (llm.Provider, error) {
		return nil, llm.ErrNoProvider
	}
}
