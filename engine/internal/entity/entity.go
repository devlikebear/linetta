// Package entity owns Entity (character/place/item/concept) domain logic.
package entity

// Kinds.
const (
	KindCharacter = "character"
	KindPlace     = "place"
	KindItem      = "item"
	KindConcept   = "concept"
)

// Entity mirrors the SQLite row. Attributes is free-form key→string JSON.
type Entity struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Aliases    []string          `json:"aliases"`
	Role       string            `json:"role"`
	Summary    string            `json:"summary"`
	Attributes map[string]string `json:"attributes"`
	CreatedAt  int64             `json:"created_at"`
	UpdatedAt  int64             `json:"updated_at"`
}

// NewInput is what `entities.create` accepts.
type NewInput struct {
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Role      string `json:"role"`
}

// UpdateInput is what `entities.update` accepts. Fields with their zero value
// are left unchanged. Use a nil map to leave attributes unchanged; use an empty
// map to clear them.
type UpdateInput struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	Name       string             `json:"name"`
	Role       string             `json:"role"`
	Summary    string             `json:"summary"`
	Attributes *map[string]string `json:"attributes,omitempty"`
}
