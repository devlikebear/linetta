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

	DefaultEpisodeCharTarget = 5000

	// A new web novel starts on the length Korean serial sites and contests
	// ask for: at least 5,000 characters with spaces, aiming a little past it
	// so an agent told "about the target" does not land under the minimum.
	DefaultWebnovelEpisodeCharMin    = 5000
	DefaultWebnovelEpisodeCharTarget = 5300
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
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Genres            []string `json:"genres"`
	LengthTarget      string   `json:"length_target"` // flash|short|novella|novel|series
	DefaultPOV        string   `json:"default_pov"`   // first|third_limited|omniscient
	StyleNotes        string   `json:"style_notes"`
	Outline           string   `json:"outline"`
	OutlinePreset     string   `json:"outline_preset"`
	EpisodeCharTarget int      `json:"episode_char_target"`
	// EpisodeCharMin and EpisodeCharMax bound an episode's length under the
	// work's basis; 0 means no bound. EpisodeCharTarget lies between them.
	EpisodeCharMin int `json:"episode_char_min"`
	EpisodeCharMax int `json:"episode_char_max"`
	// The submission rule: what a contest or serial site asks of the whole
	// work. 0 and false mean the work sets none (see ValidSubmissionRule).
	SubmissionMinEpisodes   int  `json:"submission_min_episodes"`
	SubmissionTotalMin      int  `json:"submission_total_min"`
	SubmissionTotalMax      int  `json:"submission_total_max"`
	SubmissionNeedsSynopsis bool `json:"submission_needs_synopsis"`
	// CharCountBasis is how the work counts characters: charcount.WithSpaces
	// or charcount.WithoutSpaces. WordCount and every scene's count follow it.
	CharCountBasis   string  `json:"char_count_basis"`
	Synopsis         string  `json:"synopsis"`
	WordCount        int     `json:"word_count"`
	LastOpenedNodeID *string `json:"last_opened_node_id,omitempty"`
	CreatedAt        int64   `json:"created_at"`
	UpdatedAt        int64   `json:"updated_at"`
	ArchivedAt       *int64  `json:"archived_at,omitempty"`
}

// NewInput is what the UI submits from the New Project modal.
type NewInput struct {
	Title         string   `json:"title"`
	Genres        []string `json:"genres"`
	LengthTarget  string   `json:"length_target"`
	DefaultPOV    string   `json:"default_pov"`
	OutlinePreset string   `json:"outline_preset,omitempty"`
}

// ListFilter selects which projects to return.
type ListFilter struct {
	IncludeArchived bool `json:"include_archived"`
	Limit           int  `json:"limit"`
}

// UpdateInput patches editable project fields. Each pointer field is nil to
// leave the value alone, or non-nil (including "") to set it.
type UpdateInput struct {
	ID                string    `json:"id"`
	Title             *string   `json:"title,omitempty"`
	Genres            *[]string `json:"genres,omitempty"`
	LengthTarget      *string   `json:"length_target,omitempty"`
	DefaultPOV        *string   `json:"default_pov,omitempty"`
	StyleNotes        *string   `json:"style_notes,omitempty"`
	Outline           *string   `json:"outline,omitempty"`
	OutlinePreset     *string   `json:"outline_preset,omitempty"`
	EpisodeCharTarget *int      `json:"episode_char_target,omitempty"`
	EpisodeCharMin    *int      `json:"episode_char_min,omitempty"`
	EpisodeCharMax    *int      `json:"episode_char_max,omitempty"`

	SubmissionMinEpisodes   *int    `json:"submission_min_episodes,omitempty"`
	SubmissionTotalMin      *int    `json:"submission_total_min,omitempty"`
	SubmissionTotalMax      *int    `json:"submission_total_max,omitempty"`
	SubmissionNeedsSynopsis *bool   `json:"submission_needs_synopsis,omitempty"`
	CharCountBasis          *string `json:"char_count_basis,omitempty"`
	Synopsis                *string `json:"synopsis,omitempty"`
}

// HasSubmissionRule reports whether the work sets any submission rule.
func (p Project) HasSubmissionRule() bool {
	return p.SubmissionMinEpisodes > 0 || p.SubmissionTotalMin > 0 || p.SubmissionTotalMax > 0 ||
		p.SubmissionNeedsSynopsis
}

// ValidSubmissionRule reports whether a submission rule holds together: no
// negative numbers, and the work's shortest length not past its longest.
func ValidSubmissionRule(minEpisodes, totalMin, totalMax int) bool {
	if minEpisodes < 0 || totalMin < 0 || totalMax < 0 {
		return false
	}
	return totalMax == 0 || totalMin <= totalMax
}
