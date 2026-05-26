// Package ai owns prompt assembly and run management for AI mode.
package ai

// Options is the per-call user-selected options.
type Options struct {
	TonePreset bool `json:"tone_preset"` // include style_notes prominently
	ShortForm  bool `json:"short_form"`  // ask for one-paragraph length
}

// Context is the structured payload that prompts.go renders into the
// final prompt. Stored as ai_runs.context_json so the user can later see
// exactly what was sent.
type Context struct {
	ProjectID   string        `json:"project_id"`
	NodeID      string        `json:"node_id"`
	SceneLabel  string        `json:"scene_label"`
	SceneText   string        `json:"scene_text"`
	PrevSummary string        `json:"prev_summary"`
	Entities      []EntityBrief  `json:"entities"`
	ActiveThreads []ActiveThread `json:"active_threads"`
	StyleNotes    string         `json:"style_notes"`
	UserPrompt    string         `json:"user_prompt"`
	Options       Options        `json:"options"`
}

// EntityBrief is the entity slice we send to the LLM. Just enough to ground
// dialogue / description without flooding the context.
type EntityBrief struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Role       string            `json:"role"`
	Summary    string            `json:"summary"`
	Attributes map[string]string `json:"attributes"`
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
