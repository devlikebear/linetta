// Package project owns Project domain types and the SQLite-backed Repo.
package project

// Project is the on-wire and on-disk representation of a single work.
// Genres is a JSON-array stored as TEXT in SQLite; the repo handles conversion.
type Project struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Genres           []string `json:"genres"`
	LengthTarget     string   `json:"length_target"` // flash|short|novella|novel|series
	DefaultPOV       string   `json:"default_pov"`   // first|third_limited|omniscient
	StyleNotes       string   `json:"style_notes"`
	WordCount        int      `json:"word_count"`
	LastOpenedNodeID *string  `json:"last_opened_node_id,omitempty"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
	ArchivedAt       *int64   `json:"archived_at,omitempty"`
}

// NewInput is what the UI submits from the New Project modal.
type NewInput struct {
	Title        string   `json:"title"`
	Genres       []string `json:"genres"`
	LengthTarget string   `json:"length_target"`
	DefaultPOV   string   `json:"default_pov"`
}

// ListFilter selects which projects to return.
type ListFilter struct {
	IncludeArchived bool `json:"include_archived"`
	Limit           int  `json:"limit"`
}
