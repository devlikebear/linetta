package node

const (
	KindContainer = "container"
	KindLeaf      = "leaf"

	StatusDraft     = "draft"
	StatusRevision  = "revision"
	StatusFinal     = "final"
	StatusPublished = "published"
)

func ValidKind(kind string) bool {
	return kind == KindContainer || kind == KindLeaf
}

func ValidStatus(status string) bool {
	return status == StatusDraft || status == StatusRevision || status == StatusFinal || status == StatusPublished
}

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
	SummaryWritten    bool    `json:"summary_written"` // written by an agent or an import, not derived; see SummaryUsable
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
}

// SummaryFresh reports whether Summary describes the node as it is now.
func (n Node) SummaryFresh() bool {
	return n.Summary != "" && n.SummaryForVersion == n.ContentVersion
}

// SummaryUsable is the summary a reader should be given, and whether it
// describes an earlier version of the node. A derived summary — an opening
// cut, a rollup — is only worth anything while fresh, and is rebuilt on
// demand. A written one stays the best account of the node there is until
// someone rewrites it, so it comes back flagged stale rather than dropped in
// favour of the scene's opening lines.
func (n Node) SummaryUsable() (summary string, stale bool) {
	switch {
	case n.SummaryFresh():
		return n.Summary, false
	case n.Summary != "" && n.SummaryWritten:
		return n.Summary, true
	default:
		return "", false
	}
}
