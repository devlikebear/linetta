package node

// Node mirrors the SQLite row. content_doc is the raw Tiptap JSON; the engine
// stores and serves it verbatim, never re-shaping it.
type Node struct {
	ID                string  `json:"id"`
	ProjectID         string  `json:"project_id"`
	ParentID          *string `json:"parent_id,omitempty"`
	Ordinal           int     `json:"ordinal"`
	Kind              string  `json:"kind"` // 'container' | 'leaf'
	Label             string  `json:"label"`
	Title             string  `json:"title"`
	ContentDoc        *string `json:"content_doc,omitempty"` // null for containers
	Status            string  `json:"status"`
	WordCount         int     `json:"word_count"`
	Summary           string  `json:"summary"`
	ContentVersion    int     `json:"content_version"`
	SummaryForVersion int     `json:"summary_for_version"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
}
