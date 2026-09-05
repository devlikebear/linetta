//go:build !mobile

package agent

import "fmt"

// logf writes agent diagnostics to stdout, the way mcphost does. Failures
// here are never fatal to a turn: a transcript row that did not save must not
// cost the writer the reply it belonged to.
func logf(format string, args ...any) {
	fmt.Printf("agent: "+format+"\n", args...)
}
