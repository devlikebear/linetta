// Package relationship owns Entity↔Entity edges (관계). The schema lives in
// 0001_init.sql; 0003 adds the nullable pair_id column used to group inverse
// rows so deletes can cascade to the partner.
package relationship

// Relationship mirrors the SQLite row. PairID is nil for singletons (no inverse).
type Relationship struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"project_id"`
	FromID    string  `json:"from_id"`
	ToID      string  `json:"to_id"`
	Label     string  `json:"label"`
	Notes     string  `json:"notes"`
	PairID    *string `json:"pair_id,omitempty"`
}

// NewInput is the singleton form (no inverse).
type NewInput struct {
	ProjectID string `json:"project_id"`
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	Label     string `json:"label"`
	Notes     string `json:"notes"`
}

// NewPairInput creates two rows in one transaction: (From→To label) and
// (To→From inverse_label), sharing one fresh pair_id.
type NewPairInput struct {
	ProjectID    string `json:"project_id"`
	FromID       string `json:"from_id"`
	ToID         string `json:"to_id"`
	Label        string `json:"label"`
	InverseLabel string `json:"inverse_label"`
	Notes        string `json:"notes"`
}

// UpdateInput patches a single row. The paired side keeps its own values.
type UpdateInput struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Notes string `json:"notes"`
}
