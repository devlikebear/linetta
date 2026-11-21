//go:build mas || mobile

package engineapp

import (
	"context"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

// A sandboxed or mobile build runs no other program, so it has no web search:
// the agent never turns the tool on, and the searcher refuses if asked.

func webSearchAvailable() bool { return false }

type noWebSearch struct{}

func (noWebSearch) Search(context.Context, string, string, int) (string, error) {
	return "", errors.New("web search is not available in this build")
}

func newAsideWebSearch() mcphost.WebSearcher { return noWebSearch{} }
