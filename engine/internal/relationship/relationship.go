// Package relationship owns Entity↔Entity edges (관계). The schema lives in
// 0001_init.sql; 0003 adds the nullable pair_id column used to group inverse
// rows so deletes can cascade to the partner, and 0059 the stretch of the
// story a row holds over (#386).
package relationship

// Relationship mirrors the SQLite row. PairID is nil for singletons (no inverse).
// SinceNodeID is the scene the relationship starts in (nil: from the start);
// UntilNodeID the scene its successor, the row whose SupersedesID names it,
// takes over in (nil: to the end).
type Relationship struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"project_id"`
	FromID       string  `json:"from_id"`
	ToID         string  `json:"to_id"`
	Label        string  `json:"label"`
	Notes        string  `json:"notes"`
	PairID       *string `json:"pair_id,omitempty"`
	SinceNodeID  *string `json:"since_node_id,omitempty"`
	UntilNodeID  *string `json:"until_node_id,omitempty"`
	SinceQuote   string  `json:"since_quote,omitempty"`
	SupersedesID *string `json:"supersedes_id,omitempty"`
}

// NewInput is the singleton form (no inverse).
type NewInput struct {
	ProjectID string `json:"project_id"`
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	Label     string `json:"label"`
	Notes     string `json:"notes"`
	// SinceNodeID and SinceQuote place the start in the story; empty means
	// it has held from the start.
	SinceNodeID string `json:"since_node_id,omitempty"`
	SinceQuote  string `json:"since_quote,omitempty"`
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
	SinceNodeID  string `json:"since_node_id,omitempty"`
	SinceQuote   string `json:"since_quote,omitempty"`
}

// UpdateInput patches a single row. The paired side keeps its own values.
type UpdateInput struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Notes string `json:"notes"`
}

// ChangeInput turns a relationship into another from a scene on: "동료" until
// AtNodeID, "적" from there. A paired row changes with its partner, whose new
// label is InverseLabel, or its old one when that is empty.
type ChangeInput struct {
	ID           string `json:"id"`
	AtNodeID     string `json:"at_node_id"`
	Label        string `json:"label"`
	InverseLabel string `json:"inverse_label,omitempty"`
	Quote        string `json:"quote,omitempty"`
}

// SpanInput sets where a row sits in the story, for an import putting back
// what an export wrote. Nil and empty fields clear the column.
type SpanInput struct {
	ID           string
	SinceNodeID  *string
	UntilNodeID  *string
	SinceQuote   string
	SupersedesID *string
}
