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

// Core narrative roles are always worth surfacing to a connected agent even
// when the current scene has not mentioned them yet.
//
// Entity.Role stores whatever the writer picked, in their own language, so this
// table has to hold all three. It held only Korean, which meant an English or
// Japanese writer's protagonist was never core and their story context arrived
// with no cast at all (#45) — a silent one, because nothing failed.
//
// Keep in step with entityRolePresets in apps/desktop/src/lib/i18n.tsx.
var coreRolesByKind = map[string]map[string]bool{
	KindCharacter: {
		// ko — including 적수, an older label still in existing works.
		"주인공": true, "공동 주인공": true, "조연": true, "빌런": true,
		"라이벌": true, "멘토": true, "조력자": true, "적수": true,
		// en
		"Protagonist": true, "Co-protagonist": true, "Supporting": true,
		"Villain": true, "Rival": true, "Mentor": true, "Helper": true,
		// ja
		"主人公": true, "共同主人公": true, "脇役": true, "ヴィラン": true,
		"ライバル": true, "メンター": true, "協力者": true,
	},
	KindPlace: {
		// ko — spacing variants included, since these were typed by hand
		// before they were a preset.
		"메인무대": true, "메인 무대": true, "메인무대장소": true,
		"메인 무대 장소": true, "특별한 장소": true, "일상 거점": true,
		"위험 구역": true, "금지된 장소": true, "기억의 장소": true,
		"권력의 중심": true,
		// en
		"Main stage": true, "Special place": true, "Everyday base": true,
		"Danger zone": true, "Forbidden place": true, "Place of memory": true,
		// ja
		"メイン舞台": true, "特別な場所": true, "日常の拠点": true,
		"危険区域": true, "禁じられた場所": true, "記憶の場所": true,
	},
}

// CoreRolePresetsKo returns the canonical Korean preset roles per kind — the
// ones the app's role picker offers. Legacy spellings in coreRolesByKind stay
// accepted but are not part of this list. The storyops schema guard ties the
// MCP role documentation to it, so adding a preset here without updating the
// Op.Role jsonschema tag fails a test instead of silently lying to agents.
func CoreRolePresetsKo() map[string][]string {
	return map[string][]string{
		KindCharacter: {"주인공", "공동 주인공", "조연", "빌런", "라이벌", "멘토", "조력자"},
		KindPlace:     {"메인 무대", "특별한 장소", "일상 거점", "위험 구역", "금지된 장소", "기억의 장소", "권력의 중심"},
	}
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

// UpdateInput is what `entities.update` accepts. Nil pointers leave fields
// unchanged; pointers to empty strings explicitly clear optional text fields.
// Use a nil map to leave attributes unchanged; use an empty map to clear them.
type UpdateInput struct {
	ID         string             `json:"id"`
	Kind       *string            `json:"kind,omitempty"`
	Name       *string            `json:"name,omitempty"`
	Aliases    *[]string          `json:"aliases,omitempty"`
	Role       *string            `json:"role,omitempty"`
	Summary    *string            `json:"summary,omitempty"`
	Attributes *map[string]string `json:"attributes,omitempty"`
}
