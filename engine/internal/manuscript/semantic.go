package manuscript

import "context"

// SemanticMatch is a paragraph meaning search (#384) found for a query, best
// first.
type SemanticMatch struct {
	NodeID string
	Para   int
	Text   string
	Score  float32
}

// Semantic is meaning search, when the writer has it on. ok is false when it
// cannot answer yet (off, no model, nothing embedded): the search is then
// words alone, as before.
type Semantic interface {
	Search(ctx context.Context, projectID, query string, k int) (matches []SemanticMatch, ok bool, err error)
}
