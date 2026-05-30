// Package beat owns Beat (Thread의 마디) domain logic. Beats belong to a
// Thread (cascade delete) and optionally bind to a Node (ON DELETE SET NULL).
package beat

// Beat mirrors the SQLite row. NodeID is nil when the beat is unbound or its
// bound node was deleted. Description carries the "what happens here" plot body.
type Beat struct {
	ID          string  `json:"id"`
	ThreadID    string  `json:"thread_id"`
	NodeID      *string `json:"node_id,omitempty"`
	Ordinal     int     `json:"ordinal"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Intensity   int     `json:"intensity"`
}

// NewInput is what `beats.create` accepts. Ordinal is assigned by the repo.
type NewInput struct {
	ThreadID    string  `json:"thread_id"`
	NodeID      *string `json:"node_id,omitempty"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Intensity   int     `json:"intensity"`
}

// UpdateInput is what `beats.update` accepts. Empty Label leaves the field
// alone; Intensity == 0 leaves it alone (use 1..3 to set). Description is a
// pointer: nil leaves it alone, a non-nil pointer (including "") sets it — so
// the body can be explicitly cleared.
type UpdateInput struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Description *string `json:"description,omitempty"`
	Intensity   int     `json:"intensity"`
}
