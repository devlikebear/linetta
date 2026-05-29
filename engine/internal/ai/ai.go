// Package ai owns prompt assembly and run management for AI mode.
package ai

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
	Hierarchical  HierarchicalContext `json:"hierarchical"`
	RelatedScenes []SceneSummary      `json:"related_scenes"`
	Entities      []EntityBrief       `json:"entities"`
	ActiveThreads []ActiveThread      `json:"active_threads"`
	Notes         []NoteBrief         `json:"notes"`
	StyleNotes    string              `json:"style_notes"`
	UserPrompt    string              `json:"user_prompt"`
	Options       Options             `json:"options"`
}

// SceneSummary is one rendered leaf/scene rollup (label + body). Used by both
// hierarchical layer 1 (nearby + same chapter) and topology RAG layer 3.
type SceneSummary struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"` // breadcrumb path, e.g. "1부 / 1장 / 씬 3"
	Body   string `json:"body"`
}

// ChapterSummary is the 장-level rollup body + its breadcrumb label.
type ChapterSummary struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Body   string `json:"body"`
}

// PartSummary is the 부-level rollup.
type PartSummary struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Body   string `json:"body"`
}

// HierarchicalContext is layer 1 of Plan 16's hierarchical context. Each slice
// may be empty; renderer skips empty sections.
type HierarchicalContext struct {
	NearbyLeafSummaries   []SceneSummary   `json:"nearby_leaf_summaries"`
	SameChapterSummaries  []SceneSummary   `json:"same_chapter_summaries"`
	OtherChapterSummaries []ChapterSummary `json:"other_chapter_summaries"`
	OtherPartSummaries    []PartSummary    `json:"other_part_summaries"`
	ProjectSynopsis       string           `json:"project_synopsis"`
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

// ActiveThread is an open storyline thread that touches the current node.
type ActiveThread struct {
	Name        string      `json:"name"`
	Color       string      `json:"color"`
	Summary     string      `json:"summary"`
	RecentBeats []BeatBrief `json:"recent_beats"`
}

// BeatBrief is a minimal beat representation sent to the LLM.
type BeatBrief struct {
	Label   string `json:"label"`
	Ordinal int    `json:"ordinal"`
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
