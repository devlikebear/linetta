// Package plot builds the "plot spine": the beats bound to the previous,
// current, and next scene (leaf) in document order, enriched with their
// thread's name and color. Shared by the AI context builder and the
// plot.spine_panel RPC handler so both agree on which scenes are neighbors.
package plot

// Beat is a thread beat enriched with its parent thread's display fields.
type Beat struct {
	ID          string `json:"id"`
	ThreadID    string `json:"thread_id"`
	ThreadName  string `json:"thread_name"`
	ThreadColor string `json:"thread_color"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Intensity   int    `json:"intensity"`
	Ordinal     int    `json:"ordinal"`
}

// SceneBeats is one scene (leaf) and the beats bound to it, in (thread, ordinal)
// order. Label is the breadcrumb path, e.g. "1부 / 1장 / 씬 3".
type SceneBeats struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Beats  []Beat `json:"beats"`
}

// Spine is the prev/current/next window. Prev/Next are nil at the document
// boundaries (first/last leaf). Current is always present.
type Spine struct {
	Prev    *SceneBeats `json:"prev"`
	Current SceneBeats  `json:"current"`
	Next    *SceneBeats `json:"next"`
}
