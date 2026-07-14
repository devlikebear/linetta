// Package thread owns Thread (스토리라인) domain logic. The schema already
// exists in 0001_init.sql; this package adds the Go layer.
package thread

// Thread mirrors the SQLite row. ClosedAt is nil while the thread is open.
type Thread struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Summary   string `json:"summary"`
	ClosedAt  *int64 `json:"closed_at,omitempty"`
}

// NewInput is what `threads.create` accepts.
type NewInput struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
}

// UpdateInput holds a partial patch. Nil leaves a field unchanged; a pointer to
// an empty string explicitly clears optional text.
type UpdateInput struct {
	ID      string  `json:"id"`
	Name    *string `json:"name,omitempty"`
	Color   *string `json:"color,omitempty"`
	Summary *string `json:"summary,omitempty"`
}
