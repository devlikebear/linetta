// Package mention persists per-node Entity references derived from the doc.
package mention

// Mention mirrors the mentions row, plus the surface text shown in the body.
type Mention struct {
	ID       string `json:"id"`
	NodeID   string `json:"node_id"`
	EntityID string `json:"entity_id"`
	Position int    `json:"position"`
	Surface  string `json:"surface"`
}

// Found is what the walker emits — a position+entity tuple before persistence.
type Found struct {
	EntityID string
	Position int
	Surface  string
}
