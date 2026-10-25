// Package proposals holds story-structure changes an agent suggested that
// the writer has not applied yet (story-assist Phase 3). An agent submits a
// batch of ops here instead of handing them straight to storyops; nothing
// in the batch touches Story World until the writer accepts an item, one at
// a time, through Accept — which runs it through the same storyops applier
// every other write goes through.
//
// linetta_apply_story_ops is unchanged and still exists for a small, directly
// requested change. This package exists for everything pulled from the
// manuscript, or more than a few items at once: a case the apply tool's
// immediate, barely-undoable writes were never meant to carry.
//
// See docs/plans/story-assist/phase-3-proposals-import.md.
package proposals

import (
	"context"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// Purposes a proposal batch may be submitted for.
const (
	// PurposeGeneral is an ordinary agent-initiated suggestion.
	PurposeGeneral = "general"
	// PurposeImportExtraction marks a batch drawn from the manuscript while
	// filling Story World after an import — ExtractionProgress and
	// ExtractionEstimate only count items with this purpose.
	PurposeImportExtraction = "import_extraction"
	// PurposeForecast is Phase 4's (branch-forecast op bundles). Submit
	// refuses it until that phase ships — see Submit's doc comment.
	PurposeForecast = "forecast"
)

// ValidPurpose reports whether v is a purpose this phase supports.
// purpose=forecast is deliberately NOT included: it is a real value the
// schema will eventually accept, but Phase 4 is what gives it meaning, so
// Submit refuses it by name rather than lumping it in with a garbage string.
func ValidPurpose(v string) bool {
	switch v {
	case PurposeGeneral, PurposeImportExtraction:
		return true
	default:
		return false
	}
}

// Who submitted a proposal — the same two values continuity.Source and
// mcphost.ActivityEntry.Source carry.
const (
	SourceAgent    = "agent"
	SourceExternal = "external"
)

// Statuses a proposal item can carry.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
	// StatusFailed means Accept tried this item and storyops refused it —
	// a partial-failure outcome, not a terminal one the writer cannot undo
	// by fixing the op and resubmitting (resubmission is a new proposal;
	// this package has no edit-in-place for a stored item's op, only
	// Accept's edits map for one-shot corrections at accept time).
	StatusFailed = "failed"
)

// allowedOpTypes is the non-structural, non-destructive subset of
// storyops' op catalogue this package accepts. Everything else — writing
// scene text, deleting anything, remember, or restructuring the outline —
// stays on linetta_apply_story_ops / linetta_write_scene: those either
// cannot meaningfully be "pending" (text in a document, not a row) or
// already carry their own all-or-nothing/undo-batch guarantees that this
// package's one-item-at-a-time Accept must not reinterpret (3.B.3).
// Phase 4 adds create_scene and create_outline_node for purpose=forecast only.
var allowedOpTypes = map[string]bool{
	"create_entity":       true,
	"update_entity":       true,
	"create_relationship": true,
	"create_fact_card":    true,
	"create_thread":       true,
	"update_thread":       true,
	"add_beat":            true,
	"update_beat":         true,
}

// AllowedOpTypes returns the accepted op types, sorted, for error messages.
func AllowedOpTypes() []string {
	out := make([]string, 0, len(allowedOpTypes))
	for op := range allowedOpTypes {
		out = append(out, op)
	}
	// Small, fixed set — a literal sort is clearer here than importing sort
	// for eight strings that never change shape at runtime.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// GlobalIndex marks a Problem that is not about any one item.
const GlobalIndex = -1

// Problem is one thing wrong with a submission, phrased so a model reading
// it back can act on it directly — continuity.Problem's convention.
type Problem struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// Limits. A batch this large still has to fit one tool call's response and
// one screen of the inbox; MaxEvidencePerItem matches continuity's own quote
// budget per ref.
const (
	// MaxItemsPerBatch bounds one linetta_propose_story_ops call.
	MaxItemsPerBatch = 60
	// MaxEvidencePerItem bounds one item's cited evidence.
	MaxEvidencePerItem = 3
)

// EvidenceInput is one quote an agent cites for an item, in the same shape
// continuity.Ref uses for a scene ref: a leaf scene's own paragraph number
// (as linetta_read_scene numbered=true showed it) and the exact text.
type EvidenceInput struct {
	NodeID    string `json:"node_id" jsonschema:"id of the leaf scene this evidence is from"`
	Paragraph int    `json:"paragraph" jsonschema:"paragraph number in that scene, as linetta_read_scene numbered=true showed it"`
	Quote     string `json:"quote" jsonschema:"text copied EXACTLY from that paragraph — never paraphrased or invented"`
}

// ItemInput is one op a model proposes, exactly as submitted.
type ItemInput struct {
	Op storyops.Op `json:"op"`
	// Evidence cites where this item came from; verified against the
	// scene's actual paragraphs the same way continuity.Validate verifies
	// an observation's quote.
	Evidence []EvidenceInput `json:"evidence,omitempty" jsonschema:"where this came from in the manuscript, at most 3 entries; cite it where you can"`
	// FromNodeIDs are the leaf scenes this item was drawn from — purpose
	// import_extraction's progress tracking reads this.
	FromNodeIDs []string `json:"from_node_ids,omitempty" jsonschema:"for purpose=import_extraction: the leaf scenes this item was drawn from"`
}

// SubmitInput is what linetta_propose_story_ops takes.
type SubmitInput struct {
	ProjectID string      `json:"project_id"`
	Summary   string      `json:"summary,omitempty" jsonschema:"one line describing the whole batch"`
	Purpose   string      `json:"purpose,omitempty" jsonschema:"one of: general, import_extraction"`
	Items     []ItemInput `json:"items" jsonschema:"what to propose, at most 60 per call"`
}

// Duplicate is a possible match the dedupe pass found for a create_entity
// item's name or aliases. Matched is informational only — the engine never
// merges on its own; see dedupe.go.
type Duplicate struct {
	// EntityID is set when the match is against an existing entity in this
	// work; ItemID is set instead when the match is against another
	// pending proposal item's create_entity (ItemID is empty for the
	// matched item itself inside the same Submit call, since it has no id
	// yet — the match is reported on BOTH items once they are stored).
	EntityID string `json:"entity_id,omitempty"`
	ItemID   string `json:"item_id,omitempty"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	// Matched is "exact" (normalized name/alias match) or
	// "exact_kind_mismatch" (same normalized text, different entity kind).
	Matched string `json:"matched"`
}

// ItemResult is what Accept recorded for one item: a created id, or why it
// failed.
type ItemResult struct {
	CreatedID string `json:"created_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Item is one stored proposal item.
type Item struct {
	ID          string          `json:"id"`
	ProposalID  string          `json:"proposal_id"`
	ProjectID   string          `json:"project_id"`
	Ordinal     int             `json:"ordinal"`
	Op          storyops.Op     `json:"op"`
	DependsOn   []string        `json:"depends_on"`
	Evidence    []EvidenceInput `json:"evidence"`
	FromNodeIDs []string        `json:"from_node_ids"`
	Duplicates  []Duplicate     `json:"duplicates"`
	Status      string          `json:"status"`
	Result      ItemResult      `json:"result"`
	StatusAt    int64           `json:"status_at"`
}

// Proposal is one submitted batch's header row.
type Proposal struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Purpose     string            `json:"purpose"`
	Source      string            `json:"source"`
	Summary     string            `json:"summary"`
	RefBindings map[string]string `json:"ref_bindings"`
	CreatedAt   int64             `json:"created_at"`
}

// ProposalWithItems is what List returns: a batch and its items.
type ProposalWithItems struct {
	Proposal
	Items []Item `json:"items"`
}

// SubmitResult is what Submit returns on success.
type SubmitResult struct {
	ProposalID         string `json:"proposal_id"`
	Items              []Item `json:"items"`
	PossibleDuplicates int    `json:"possible_duplicates"`
}

// Service applies the propose/accept/reject lifecycle over its own storage
// (story_proposals, story_proposal_items) plus the repos it needs to check a
// ref actually belongs to the work and to apply an accepted item.
type Service struct {
	store    *store.Store
	storyops *storyops.Service
	entities *entity.Repo
	threads  *thread.Repo
	beats    *beat.Repo
	facts    *fact.Repo
	nodes    *node.Repo
	projects *project.Repo

	// notify, when set, is called after Accept/Reject change a proposal's
	// status — the Go -> ffi.rs -> frontend "proposals.changed" path
	// (3.B.3's decision: proposal status changes are their own event, not
	// mcp.changed, since "mcp" means the change came from an MCP client and
	// the writer's own accept/reject click is not that).
	notify func(method string, params any)
}

// New wires the service. notify may be nil (tests, a build with no
// notifier); every other argument is required for Submit/Accept to do
// anything.
func New(
	s *store.Store, so *storyops.Service,
	entities *entity.Repo, threads *thread.Repo, beats *beat.Repo, facts *fact.Repo,
	nodes *node.Repo, projects *project.Repo,
	notify func(method string, params any),
) *Service {
	return &Service{
		store: s, storyops: so,
		entities: entities, threads: threads, beats: beats, facts: facts,
		nodes: nodes, projects: projects,
		notify: notify,
	}
}

func (s *Service) fireNotify(method string, params any) {
	if s.notify != nil {
		s.notify(method, params)
	}
}

// episodePreset looks up the work's outline preset for episode.List —
// ExtractionProgress and ExtractionEstimate's episode breakdown needs it,
// and dedupe/validation never do, so this is only called from progress.go.
func (s *Service) episodePreset(ctx context.Context, projectID string) (string, error) {
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	return p.OutlinePreset, nil
}

// episodesFor is episode.List, named locally so progress.go reads as "this
// package's episodes" rather than repeating the import alias at every call.
func (s *Service) episodesFor(ctx context.Context, projectID string) ([]episode.Episode, error) {
	preset, err := s.episodePreset(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return episode.List(ctx, s.nodes, projectID, preset)
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
