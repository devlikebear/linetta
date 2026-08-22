// Package storycontext assembles the curated story brief for one scene:
// outline, hierarchical summaries, entity/relationship briefs, plot spine,
// notes, and style targets. It performs no LLM calls and must not import
// LLM client code; renderers return plain strings.
package storycontext

import "github.com/devlikebear/linetta/engine/internal/plot"

// Tone preset identifiers. `my` keeps the existing style_notes-driven behavior;
// the others append a fixed tone instruction to the system prompt. Empty string
// means no tone fragment at all.
const (
	TonePresetMy      = "my"
	TonePresetCool    = "cool"
	TonePresetSensory = "sensory"
	TonePresetDry     = "dry"
	TonePresetTense   = "tense"
	TonePresetLyrical = "lyrical"
	TonePresetHumor   = "humor"
)

// Options is the per-call user-selected options.
type Options struct {
	Tone             string           `json:"tone"`       // tone preset id; see TonePreset* constants
	ShortForm        bool             `json:"short_form"` // ask for one-paragraph length
	Context          ContextSelection `json:"context,omitempty"`
	OutlineStructure string           `json:"outline_structure,omitempty"`
	Language         string           `json:"language,omitempty"` // app UI language; "en*" switches prompts to English (Korean default)
}

// ContextKey identifies one independently toggleable context section.
type ContextKey string

const (
	ContextKeyCurrentScene  ContextKey = "current_scene"
	ContextKeyOverview      ContextKey = "overview"
	ContextKeySynopsis      ContextKey = "synopsis"
	ContextKeyNearbyScenes  ContextKey = "nearby_scenes"
	ContextKeyRelatedScenes ContextKey = "related_scenes"
	ContextKeyPlot          ContextKey = "plot"
	ContextKeyEntities      ContextKey = "entities"
	ContextKeyRelationships ContextKey = "relationships"
	ContextKeyNotes         ContextKey = "notes"
	ContextKeyProjectMeta   ContextKey = "project_meta"
	ContextKeyStyleNotes    ContextKey = "style_notes"
	ContextKeyFacts         ContextKey = "facts"
	ContextKeyMemories      ContextKey = "memories"
	ContextKeyReferences    ContextKey = "references"
)

// ContextSelection mirrors the AI panel checklist. Nil means "use the default",
// which is enabled for backwards compatibility with older callers.
type ContextSelection struct {
	CurrentScene  *bool `json:"current_scene,omitempty"`
	Overview      *bool `json:"overview,omitempty"`
	Synopsis      *bool `json:"synopsis,omitempty"`
	NearbyScenes  *bool `json:"nearby_scenes,omitempty"`
	RelatedScenes *bool `json:"related_scenes,omitempty"`
	Plot          *bool `json:"plot,omitempty"`
	Entities      *bool `json:"entities,omitempty"`
	Relationships *bool `json:"relationships,omitempty"`
	Notes         *bool `json:"notes,omitempty"`
	ProjectMeta   *bool `json:"project_meta,omitempty"`
	StyleNotes    *bool `json:"style_notes,omitempty"`
	Facts         *bool `json:"facts,omitempty"`
	Memories      *bool `json:"memories,omitempty"`
	References    *bool `json:"references,omitempty"`
}

// DefaultContextSelection returns the nil-pointer default: every section is on.
func DefaultContextSelection() ContextSelection { return ContextSelection{} }

func (s ContextSelection) Enabled(key ContextKey) bool {
	switch key {
	case ContextKeyCurrentScene:
		return enabledByDefault(s.CurrentScene)
	case ContextKeyOverview:
		return enabledByDefault(s.Overview)
	case ContextKeySynopsis:
		return enabledByDefault(s.Synopsis)
	case ContextKeyNearbyScenes:
		return enabledByDefault(s.NearbyScenes)
	case ContextKeyRelatedScenes:
		return enabledByDefault(s.RelatedScenes)
	case ContextKeyPlot:
		return enabledByDefault(s.Plot)
	case ContextKeyEntities:
		return enabledByDefault(s.Entities)
	case ContextKeyRelationships:
		return enabledByDefault(s.Relationships)
	case ContextKeyNotes:
		return enabledByDefault(s.Notes)
	case ContextKeyProjectMeta:
		return enabledByDefault(s.ProjectMeta)
	case ContextKeyStyleNotes:
		return enabledByDefault(s.StyleNotes)
	case ContextKeyFacts:
		return enabledByDefault(s.Facts)
	case ContextKeyMemories:
		return enabledByDefault(s.Memories)
	case ContextKeyReferences:
		return enabledByDefault(s.References)
	default:
		return true
	}
}

func enabledByDefault(v *bool) bool {
	return v == nil || *v
}

// ProjectMeta carries the project-level configuration the user set when
// creating the project (장르, 예상 분량, 기본 시점). Renders as a single
// line near the top of the user message so the LLM understands the project's
// fundamental constraints.
type ProjectMeta struct {
	Genres       []string `json:"genres"`
	LengthTarget string   `json:"length_target"`
	DefaultPOV   string   `json:"default_pov"`
	Synopsis     string   `json:"synopsis"`
}

// PreviewCounts is the structural summary of a built Context, used by the
// frontend's AIContextChecklist to honestly display what will be sent to the
// LLM before the user runs a generation.
type PreviewCounts struct {
	NearbyScenes      int  `json:"nearby_scenes"`
	HasOutline        bool `json:"has_outline"`
	HasSynopsis       bool `json:"has_synopsis"`
	RelatedScenes     int  `json:"related_scenes"`
	Entities          int  `json:"entities"`
	Relationships     int  `json:"relationships"`
	PlotBeats         int  `json:"plot_beats"`
	Notes             int  `json:"notes"`
	ProjectMetaFields int  `json:"project_meta_fields"`
	HasStyleNotes     bool `json:"has_style_notes"`
}

// PreviewSection is one context section shown in the AI panel before a run.
type PreviewSection struct {
	ID            ContextKey `json:"id"`
	Label         string     `json:"label"`
	Present       bool       `json:"present"`
	Selected      bool       `json:"selected"`
	Count         int        `json:"count"`
	Preview       string     `json:"preview"`
	CharCount     int        `json:"char_count"`
	TokenEstimate int        `json:"token_estimate"`
}

// ContextPreview keeps the historical top-level counts and adds inspectable
// sections so the UI can show what is actually being injected.
type ContextPreview struct {
	PreviewCounts
	Sections              []PreviewSection `json:"sections"`
	SelectedItemCount     int              `json:"selected_item_count"`
	SelectedCharCount     int              `json:"selected_char_count"`
	SelectedTokenEstimate int              `json:"selected_token_estimate"`
	BudgetTokenEstimate   int              `json:"budget_token_estimate"`
}

// Context is the structured payload that prompts.go renders into the
// final prompt. Stored as ai_runs.context_json so the user can later see
// exactly what was sent.
type Context struct {
	ProjectID     string              `json:"project_id"`
	NodeID        string              `json:"node_id"`
	SceneLabel    string              `json:"scene_label"`
	SceneText     string              `json:"scene_text"`
	PrevSummary   string              `json:"prev_summary"`
	Project       ProjectMeta         `json:"project"`
	Outline       string              `json:"outline"`
	Hierarchical  HierarchicalContext `json:"hierarchical"`
	RelatedScenes []SceneSummary      `json:"related_scenes"`
	Entities      []EntityBrief       `json:"entities"`
	Relationships []RelationBrief     `json:"relationships"`
	Plot          plot.Spine          `json:"plot"`
	Notes         []NoteBrief         `json:"notes"`
	Facts         []FactBrief         `json:"facts,omitempty"`
	Memories      []string            `json:"memories,omitempty"`
	References    []ReferenceBrief    `json:"references,omitempty"`
	StyleNotes    string              `json:"style_notes"`
	SelectionText string              `json:"selection_text"`
	UserPrompt    string              `json:"user_prompt"`
	Options       Options             `json:"options"`
}

// RelationBrief is one relationship between two entities present in the current
// scene. Bidirectional pairs render with "↔", singletons with "→".
type RelationBrief struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Label         string `json:"label"`
	Notes         string `json:"notes"`
	Bidirectional bool   `json:"bidirectional"`
}

// SceneSummary is one rendered leaf/scene rollup (label + body). Used by the
// nearby adjacent-scene excerpts and the topology RAG related-scenes.
type SceneSummary struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"` // breadcrumb path, e.g. "1부 / 1장 / 씬 3"
	Body   string `json:"body"`
}

// HierarchicalContext is layer 1 of Plan 16's hierarchical context. Each slice
// may be empty; renderer skips empty sections.
type HierarchicalContext struct {
	NearbyLeafSummaries []SceneSummary `json:"nearby_leaf_summaries"`
	ProjectSynopsis     string         `json:"project_synopsis"`
}

// NoteBrief is a margin-note line sent to the LLM. Anchor stays in the JSON
// payload (for ai_runs inspection) but is omitted from the prompt text.
type NoteBrief struct {
	Anchor int    `json:"anchor"`
	Body   string `json:"body"`
}

// EntityBrief is the entity slice we send to the LLM. Just enough to ground
// dialogue / description without flooding the context.
type EntityBrief struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Role       string            `json:"role"`
	Summary    string            `json:"summary"`
	Attributes map[string]string `json:"attributes"`
	Recent     []string          `json:"recent"` // Plan 16 layer 2 dossier — first lines of latest 5 leaf summaries
}

// FactBrief is one Fact Book card slice for the brief: the claim, its
// verification status, and the sources backing it.
type FactBrief struct {
	ID       string            `json:"id"`
	Status   string            `json:"status"`
	Claim    string            `json:"claim"`
	Category string            `json:"category,omitempty"`
	Result   string            `json:"result,omitempty"`
	Sources  []FactSourceBrief `json:"sources,omitempty"`
}

// FactSourceBrief is one source line under a fact card.
type FactSourceBrief struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url"`
}

// ReferenceBrief is one writer-supplied reference: purpose-labelled material
// the writer attached for the current request.
type ReferenceBrief struct {
	Title   string `json:"title"`
	Purpose string `json:"purpose,omitempty"`
	Body    string `json:"body"`
}
