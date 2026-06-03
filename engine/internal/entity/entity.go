// Package entity owns Entity (character/place/item/concept) domain logic.
package entity

import "strings"

// Kinds.
const (
	KindCharacter = "character"
	KindPlace     = "place"
	KindItem      = "item"
	KindConcept   = "concept"
)

// Core narrative roles are always worth surfacing to AI context even when the
// current scene has not mentioned them yet. These labels stay in Korean because
// they are writer-facing values stored in Entity.Role.
var coreRolesByKind = map[string]map[string]bool{
	KindCharacter: {
		"주인공": true, "공동 주인공": true, "조연": true, "빌런": true,
		"라이벌": true, "멘토": true, "조력자": true, "적수": true,
	},
	KindPlace: {
		"메인무대": true, "메인 무대": true, "메인무대장소": true,
		"메인 무대 장소": true, "특별한 장소": true, "일상 거점": true,
		"위험 구역": true, "금지된 장소": true, "기억의 장소": true,
		"권력의 중심": true,
	},
}

// IsCoreRole reports whether a kind/role pair should be treated as part of the
// story bible skeleton rather than ordinary local scene context.
func IsCoreRole(kind, role string) bool {
	kind = strings.TrimSpace(kind)
	role = strings.TrimSpace(role)
	if role == "" {
		return false
	}
	roles := coreRolesByKind[kind]
	return roles[role]
}

// IsCoreEntity reports whether e carries a core narrative role.
func IsCoreEntity(e Entity) bool {
	return IsCoreRole(e.Kind, e.Role)
}

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
