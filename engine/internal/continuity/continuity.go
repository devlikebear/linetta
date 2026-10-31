// Package continuity holds an agent's continuity reading of one scene
// (story-assist Phase 2): places where the prose may disagree with the
// work's own record — Story World, fact cards, earlier scenes, or the
// writer's own direction — or leaves something undecided. Nothing here is
// canon, and nothing here changes the manuscript; it is a list the writer
// reviews in Story World. See the README's "Continuity checks" section.
package continuity

// Limits bound what one report may contain. Validate enforces them at the
// tool boundary, the same way direction.MaxGoalRunes and style.MaxProfileRunes
// bound a model's other free text: a field with no limit is a field an
// agent can use to flood its own context — and the writer's screen — with
// one giant report.
const (
	// MaxObservationsPerReport bounds how many findings one
	// linetta_report_continuity call may record.
	MaxObservationsPerReport = 20
	// MaxSummaryRunes bounds one observation's summary.
	MaxSummaryRunes = 300
	// MaxSuggestionRunes bounds one observation's suggestion.
	MaxSuggestionRunes = 300
	// MaxQuoteRunes bounds one observation's quote, and a scene ref's quote.
	MaxQuoteRunes = 300
	// MaxRefsPerObservation bounds how many refs one observation may cite.
	MaxRefsPerObservation = 5
	// MaxNoteRunes bounds a check's free-text note.
	MaxNoteRunes = 500
)

// Categories: what kind of record an observation is about. category=direction
// is the writer's own scene intent (keep/avoid, direction.SceneIntent) or
// current focus, carried over from story-assist Phase 1.
const (
	CategoryCharacter    = "character"
	CategoryPlace        = "place"
	CategoryItem         = "item"
	CategoryRelationship = "relationship"
	CategoryTimeline     = "timeline"
	CategoryFact         = "fact"
	CategoryPlot         = "plot"
	CategoryDirection    = "direction"
	// A reader's-eye reading adds these: not a clash with the record, but a
	// place the story stops working on the page.
	CategoryMotive      = "motive"
	CategoryReadability = "readability"
	CategoryPacing      = "pacing"
	CategoryOther       = "other"
)

// ValidCategory reports whether v is one of the categories above.
func ValidCategory(v string) bool {
	switch v {
	case CategoryCharacter, CategoryPlace, CategoryItem, CategoryRelationship,
		CategoryTimeline, CategoryFact, CategoryPlot, CategoryDirection,
		CategoryMotive, CategoryReadability, CategoryPacing, CategoryOther:
		return true
	default:
		return false
	}
}

// Kinds: what sort of finding an observation is.
const (
	// KindContradiction: the prose does not agree with the work's own
	// record. Must cite at least one ref — what it contradicts.
	KindContradiction = "contradiction"
	// KindGap: the prose establishes something new the record does not
	// have. Whether to register it is the writer's call.
	KindGap = "gap"
	// KindQuestion: an ambiguous passage that needs a judgement call.
	KindQuestion = "question"
)

// ValidKind reports whether v is one of the kinds above.
func ValidKind(v string) bool {
	switch v {
	case KindContradiction, KindGap, KindQuestion:
		return true
	default:
		return false
	}
}

// Severities.
const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
	SeverityLow    = "low"
)

// ValidSeverity reports whether v is one of the severities above.
func ValidSeverity(v string) bool {
	switch v {
	case SeverityHigh, SeverityMedium, SeverityLow:
		return true
	default:
		return false
	}
}

// Statuses an observation can carry.
const (
	// StatusOpen is the status every new observation is recorded with.
	StatusOpen = "open"
	// StatusDismissed means the writer looked and decided it is not worth
	// acting on. SetStatus can still move it to resolved; Record never
	// touches a dismissed observation again.
	StatusDismissed = "dismissed"
	// StatusResolved means the writer addressed it (in the manuscript, or by
	// deciding the record itself was wrong).
	StatusResolved = "resolved"
	// StatusReplaced means a later check of the same scene superseded it.
	// Only Record sets this; SetStatus refuses to move anything into or out
	// of it.
	StatusReplaced = "replaced"
)

// ValidStatus reports whether v is one of the statuses above — the
// fact.ValidStatus convention.
func ValidStatus(v string) bool {
	switch v {
	case StatusOpen, StatusDismissed, StatusResolved, StatusReplaced:
		return true
	default:
		return false
	}
}

// Ref types: what kind of thing a ref points at.
const (
	RefTypeEntity = "entity"
	RefTypeFact   = "fact"
	RefTypeBeat   = "beat"
	RefTypeScene  = "scene"
)

// ValidRefType reports whether v is one of the ref types above.
func ValidRefType(v string) bool {
	switch v {
	case RefTypeEntity, RefTypeFact, RefTypeBeat, RefTypeScene:
		return true
	default:
		return false
	}
}

// Ref is one thing an observation points at — engine-verified, so a ref in
// storage always resolves to something this work actually has (Validate's
// whole job). Paragraph and Quote apply only to RefTypeScene: an earlier
// scene's own paragraph and the text copied from it, checked by the same
// rule as the observation's own quote so an agent cannot invent what an
// earlier chapter said any more than it can invent what the current one
// says.
type Ref struct {
	Type string `json:"type" jsonschema:"one of: entity, fact, beat, scene"`
	ID   string `json:"id" jsonschema:"the id of the entity, fact card, beat, or scene (node) this observation points at"`
	// Paragraph and Quote apply only when type is scene: the earlier scene's
	// own paragraph number (as linetta_read_scene numbered=true would show
	// it) and the exact text copied from it.
	Paragraph int    `json:"paragraph,omitempty" jsonschema:"for type=scene only: the paragraph number in THAT scene, read numbered"`
	Quote     string `json:"quote,omitempty" jsonschema:"for type=scene only: text copied exactly from that scene's paragraph"`
}

// ObservationInput is one finding exactly as a model reports it, before
// Validate and Record give it an id, a check, and a status.
type ObservationInput struct {
	Category      string `json:"category" jsonschema:"one of: character, place, item, relationship, timeline, fact, plot, direction, motive, readability, pacing, other — direction means the writer's own scene intent or current focus"`
	Kind          string `json:"kind" jsonschema:"one of: contradiction (disagrees with the work's own record — must cite a ref), gap (establishes something new; the writer decides whether to register it), question (an ambiguous passage)"`
	Severity      string `json:"severity" jsonschema:"one of: high, medium, low"`
	Summary       string `json:"summary" jsonschema:"what you found, in one or two sentences, at most 300 characters"`
	Suggestion    string `json:"suggestion,omitempty" jsonschema:"optional: how it might be fixed, at most 300 characters — not an instruction to fix it yourself"`
	ParagraphFrom int    `json:"paragraph_from" jsonschema:"first paragraph number this observation is about, as linetta_read_scene numbered=true showed it"`
	ParagraphTo   int    `json:"paragraph_to" jsonschema:"last paragraph number (equal to paragraph_from for a single paragraph)"`
	Quote         string `json:"quote" jsonschema:"text copied EXACTLY from those numbered paragraphs, at most 300 characters — never paraphrased or invented"`
	Refs          []Ref  `json:"refs,omitempty" jsonschema:"what this observation points at — an entity, fact, beat, or earlier scene this work actually has; required for kind=contradiction"`
}

// Observation is one stored finding.
type Observation struct {
	ID        string `json:"id"`
	CheckID   string `json:"check_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
	ObservationInput
	Status    string `json:"status"`
	StatusAt  int64  `json:"status_at"`
	CreatedAt int64  `json:"created_at"`
}

// Source names who recorded a check: SourceAgent (the built-in panel) or
// SourceExternal (an MCP client over HTTP) — the same two values
// mcphost.ActivityEntry.Source carries, duplicated here rather than imported
// so this package stays free of the //go:build !mobile tool layer.
const (
	SourceAgent    = "agent"
	SourceExternal = "external"
)

// Check is one continuity reading of a scene: when, by whom, at what
// content_version, and anything free-text the agent wants to add.
type Check struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	NodeID         string `json:"node_id"`
	ContentVersion int    `json:"content_version"`
	Source         string `json:"source"`
	Note           string `json:"note"`
	CheckedAt      int64  `json:"checked_at"`
}

// CheckWithObservations is what Repo.List returns: a check, its
// observations, and whether the scene's text has moved on since the check
// was recorded.
type CheckWithObservations struct {
	Check
	Observations []Observation `json:"observations"`
	// Stale is true once the scene's current content_version no longer
	// matches the version this check was recorded at.
	Stale bool `json:"stale"`
}

// ReportInput is what linetta_report_continuity takes: one scene, the
// version it was read at, and what the agent found. An empty Observations
// slice is a clean check, not an error — "I looked and found nothing" is a
// result worth keeping.
type ReportInput struct {
	ProjectID    string             `json:"project_id,omitempty" jsonschema:"optional; checked against the scene's work when given"`
	NodeID       string             `json:"node_id" jsonschema:"id of the scene this check is about"`
	SceneVersion int                `json:"scene_version" jsonschema:"the content_version linetta_read_scene returned when you read it; refused if the scene changed since"`
	Observations []ObservationInput `json:"observations" jsonschema:"what you found; an empty list records a clean check — that is a valid result, not a failure"`
	Note         string             `json:"note,omitempty" jsonschema:"free-text note about the check itself (what you compared it against), at most 500 characters"`
}

// ReportStatusWaitingForWriter is the only status Record's Report carries:
// nothing an agent does moves a recorded check further than this.
const ReportStatusWaitingForWriter = "waiting_for_writer"

// Report is what Repo.Record returns.
type Report struct {
	CheckID  string `json:"check_id"`
	Recorded int    `json:"recorded"`
	Status   string `json:"status"`
}

// Selection names observations for one bulk decision, within one work: the
// named observations, plus those of the named checks a decision can apply to.
type Selection struct {
	ProjectID      string
	ObservationIDs []string
	CheckIDs       []string
}

// Outcome reports a bulk decision: what it changed, and how many named items
// it left alone (unknown, replaced by a later check, or already there).
type Outcome struct {
	Changed int `json:"changed"`
	Skipped int `json:"skipped"`
}

// ListFilter scopes Repo.List.
type ListFilter struct {
	ProjectID string
	NodeID    string
	Statuses  []string
}
