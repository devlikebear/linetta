// Package draft stores the writer's work-in-progress posts.
package draft

import "errors"

var (
	// ErrNotFound means no draft has the given id.
	ErrNotFound = errors.New("draft not found")
	// ErrConflict means the caller's content_version is stale.
	ErrConflict = errors.New("draft content_version conflict")
)

// Draft is a stored draft. Timestamps are Unix milliseconds.
type Draft struct {
	ID             int64  `json:"id"`
	Body           string `json:"body"`
	Platform       string `json:"platform"`
	Kind           string `json:"kind"`
	ContentVersion int64  `json:"content_version"`
	PublishedAt    int64  `json:"published_at"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}
