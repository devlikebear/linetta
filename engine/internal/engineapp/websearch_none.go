//go:build mas || mobile

package engineapp

import "github.com/devlikebear/linetta/engine/internal/mcphost"

// A sandboxed or mobile build runs no other program, so it has no web search.

func webSearchAvailable() bool { return false }

func newAsideWebSearch() mcphost.WebSearcher { return nil }
