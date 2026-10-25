package forecast

// Limits. Phase 4 forces these in code — InkOS leaves the 2–5 / 1–10 range
// to the tool description only (docs/plans/story-assist/inkos-analysis.md
// §3's "리네타가 더 잘할 수 있는 점").
const (
	MinBranches = 2
	MaxBranches = 5
	MinHorizon  = 1
	MaxHorizon  = 10

	MaxDivergenceRunes = 300
	MaxTitleRunes      = 80
	MaxPremiseRunes    = 600

	MinBeatsPerBranch   = 1
	MaxBeatsPerBranch   = 10
	MaxBeatSummaryRunes = 240

	MaxDecisionsPerBranch = 6
	MaxDecisionRunes      = 300

	MaxChangeItemsPerCategory = 5
	MaxChangeItemRunes        = 200

	MaxRisksPerBranch       = 5
	MaxRiskDescriptionRunes = 300

	MaxUnknownsPerBranch = 5
	MaxUnknownRunes      = 200

	MaxDirectionFitRunes = 300
)

// RiskKind is the closed set linetta_save_forecast's risks[].kind accepts.
const (
	RiskContinuity = "continuity"
	RiskCausality  = "causality"
	RiskCharacter  = "character"
	RiskDirection  = "direction"
	RiskPacing     = "pacing"
	RiskOther      = "other"
)

// ValidRiskKind reports whether v is one of the closed risk kinds.
func ValidRiskKind(v string) bool {
	switch v {
	case RiskContinuity, RiskCausality, RiskCharacter, RiskDirection, RiskPacing, RiskOther:
		return true
	default:
		return false
	}
}

// SourceAgent and SourceExternal mirror continuity/proposals' own source
// values: who saved the forecast.
const (
	SourceAgent    = "agent"
	SourceExternal = "external"
)

// BranchBeat is one episode-sized beat of a branch's projected course —
// InkOS's beats:[{chapter,summary}], renamed to Linetta's own "episode"
// vocabulary.
type BranchBeat struct {
	// EpisodeOffset counts forward from the forecast's base episode: 1 is
	// the very next episode, up to Horizon.
	EpisodeOffset int    `json:"episode_offset" jsonschema:"1..horizon: how many episodes ahead of the base episode this beat falls"`
	Summary       string `json:"summary" jsonschema:"what happens in this beat, up to 240 characters"`
}

// CharDecision is one character's choice at the branch point — InkOS's
// characterDecisions:[{character,decision}]. EntityID is validated against
// the work, the same strictness Phase 3's proposals apply to every ref.
type CharDecision struct {
	EntityID string `json:"entity_id" jsonschema:"id of an existing character in this work"`
	Decision string `json:"decision" jsonschema:"what they decide or do, up to 300 characters"`
}

// ProjectedChanges is what the branch would leave different in Story World
// if it played out — InkOS's projectedChanges, with "hooks" renamed to
// Linetta's own "threads" concept.
type ProjectedChanges struct {
	Characters    []string `json:"characters,omitempty" jsonschema:"at most 5 short notes on how characters would change"`
	Relationships []string `json:"relationships,omitempty" jsonschema:"at most 5 short notes on how relationships would change"`
	World         []string `json:"world,omitempty" jsonschema:"at most 5 short notes on how Story World elements would change"`
	Threads       []string `json:"threads,omitempty" jsonschema:"at most 5 short notes on how storylines would change"`
}

// Risk is one way the branch could go wrong, naming the kind and — where
// it applies — the specific entity, fact or scene the risk is about, so
// the writer can jump straight to it. At most one of EntityID/FactID/NodeID
// is normally set, but nothing requires exactly one.
type Risk struct {
	Kind        string `json:"kind" jsonschema:"one of: continuity, causality, character, direction, pacing, other"`
	Description string `json:"description" jsonschema:"up to 300 characters"`
	EntityID    string `json:"entity_id,omitempty" jsonschema:"id of an existing character this risk is about, if any"`
	FactID      string `json:"fact_id,omitempty" jsonschema:"id of an existing fact card this risk is about, if any"`
	NodeID      string `json:"node_id,omitempty" jsonschema:"id of an existing scene this risk is about, if any"`
}

// BranchInput is what linetta_save_forecast submits for one branch —
// everything Branch has except ID, which the engine assigns.
type BranchInput struct {
	Title        string           `json:"title" jsonschema:"up to 80 characters"`
	Premise      string           `json:"premise" jsonschema:"up to 600 characters"`
	Beats        []BranchBeat     `json:"beats" jsonschema:"1..10 beats, in order"`
	Decisions    []CharDecision   `json:"decisions,omitempty" jsonschema:"at most 6"`
	Changes      ProjectedChanges `json:"changes,omitempty"`
	Risks        []Risk           `json:"risks,omitempty" jsonschema:"at most 5; cite conflicts with the work's own record here"`
	Unknowns     []string         `json:"unknowns,omitempty" jsonschema:"at most 5 open questions this branch leaves, each up to 200 characters"`
	DirectionFit string           `json:"direction_fit,omitempty" jsonschema:"up to 300 characters: how this branch serves the writer's current focus and intent"`
}

// Branch is one stored continuation: a BranchInput plus its engine-assigned
// id (b1..bN, in submission order).
type Branch struct {
	ID string `json:"id"`
	BranchInput
}

// SaveInput is what Save validates and, if clean, stores.
type SaveInput struct {
	ProjectID  string
	Divergence string
	Horizon    int
	Branches   []BranchInput
}

// Forecast is one stored comparison: the question, the branches drawn from
// it, and the canon fingerprint it was drawn from. Nothing here is written
// back into the outline, the manuscript or Story World — see this
// package's doc comment.
type Forecast struct {
	ID             string      `json:"id"`
	ProjectID      string      `json:"project_id"`
	Divergence     string      `json:"divergence"`
	BaseNodeID     string      `json:"base_node_id,omitempty"`
	Horizon        int         `json:"horizon"`
	Branches       []Branch    `json:"branches"`
	Fingerprint    string      `json:"fingerprint"`
	InputHashes    InputHashes `json:"input_hashes"`
	Source         string      `json:"source"`
	SelectedBranch string      `json:"selected_branch,omitempty"`
	SelectedAt     int64       `json:"selected_at,omitempty"`
	CreatedAt      int64       `json:"created_at"`
}

// Selection returns the forecast's currently selected branch and whether
// one is selected at all.
func (f Forecast) Selection() (Branch, bool) {
	for _, b := range f.Branches {
		if b.ID == f.SelectedBranch {
			return b, true
		}
	}
	return Branch{}, false
}

// ListedForecast is one row of forecast.list: a Forecast plus whether it is
// stale against the project's CURRENT canon, recomputed on every list call
// — never cached, and never a one-way flag the way InkOS's is (§3's "한
// 번 낡으면 되돌릴 수 없다" is the thing Phase 4 does not repeat).
type ListedForecast struct {
	Forecast
	Stale   bool          `json:"stale"`
	Reasons []StaleReason `json:"reasons,omitempty"`
}
