// Package ai owns prompt assembly and run management for AI mode.
package ai

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
	Tone      string `json:"tone"`       // tone preset id; see TonePreset* constants
	ShortForm bool   `json:"short_form"` // ask for one-paragraph length
}

// ProjectMeta carries the project-level configuration the user set when
// creating the project (장르, 예상 분량, 기본 시점). Renders as a single
// line near the top of the user message so the LLM understands the project's
// fundamental constraints.
type ProjectMeta struct {
	Genres       []string `json:"genres"`
	LengthTarget string   `json:"length_target"`
	DefaultPOV   string   `json:"default_pov"`
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

// DeltaPayload is the body of an "ai.delta" notification.
type DeltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}

// DonePayload is the body of an "ai.done" notification.
type DonePayload struct {
	RunID    string `json:"run_id"`
	FullText string `json:"full_text"`
}

// ErrorPayload is the body of an "ai.error" notification.
type ErrorPayload struct {
	RunID   string `json:"run_id"`
	Message string `json:"message"`
}

// CancelledPayload is the body of an "ai.cancelled" notification.
type CancelledPayload struct {
	RunID string `json:"run_id"`
}

// ResetPayload is the body of an "ai.reset" notification. Sent when the
// streaming text needs to be REPLACED (not appended) — used when the upstream
// provider's transparent retry produces deltas that diverge from earlier ones
// and we need to reconcile the frontend's view to the deduplicated buffer.
type ResetPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}
