// Package project owns Project domain types and the SQLite-backed Repo.
package project

const (
	LengthFlash   = "flash"
	LengthShort   = "short"
	LengthNovella = "novella"
	LengthNovel   = "novel"
	LengthSeries  = "series"

	POVFirst        = "first"
	POVThirdLimited = "third_limited"
	POVOmniscient   = "omniscient"

	OutlinePresetNovel    = "novel"
	OutlinePresetWebNovel = "webnovel"
)

func ValidLengthTarget(v string) bool {
	switch v {
	case LengthFlash, LengthShort, LengthNovella, LengthNovel, LengthSeries:
		return true
	default:
		return false
	}
}

func ValidDefaultPOV(v string) bool {
	switch v {
	case POVFirst, POVThirdLimited, POVOmniscient:
		return true
	default:
		return false
	}
}

func ValidOutlinePreset(v string) bool {
	switch v {
	case OutlinePresetNovel, OutlinePresetWebNovel:
		return true
	default:
		return false
	}
}

// Project is the on-wire and on-disk representation of a single work.
// Genres is a JSON-array stored as TEXT in SQLite; the repo handles conversion.
type Project struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Genres           []string `json:"genres"`
	LengthTarget     string   `json:"length_target"` // flash|short|novella|novel|series
	DefaultPOV       string   `json:"default_pov"`   // first|third_limited|omniscient
	StyleNotes       string   `json:"style_notes"`
	Outline          string   `json:"outline"`
	OutlinePreset    string   `json:"outline_preset"`
	Synopsis         string   `json:"synopsis"`
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

// UpdateInput patches editable project fields. Each pointer field is nil to
// leave the value alone, or non-nil (including "") to set it.
type UpdateInput struct {
	ID            string  `json:"id"`
	Title         *string `json:"title,omitempty"`
	Outline       *string `json:"outline,omitempty"`
	OutlinePreset *string `json:"outline_preset,omitempty"`
	Synopsis      *string `json:"synopsis,omitempty"`
}
