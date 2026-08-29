package entity

import "testing"

func TestIsCoreRole_recognisesEveryLanguageTheAppOffers(t *testing.T) {
	// Entity.Role holds whatever the writer picked, in their own language.
	// The table used to hold only Korean, so an English or Japanese writer's
	// protagonist was never core and their story context arrived with no cast
	// (#45). Nothing failed — it just came back empty.
	characters := []string{
		"주인공", "공동 주인공", "조연", "빌런", "라이벌", "멘토", "조력자",
		"Protagonist", "Co-protagonist", "Supporting", "Villain", "Rival", "Mentor", "Helper",
		"主人公", "共同主人公", "脇役", "ヴィラン", "ライバル", "メンター", "協力者",
	}
	for _, role := range characters {
		if !IsCoreRole(KindCharacter, role) {
			t.Errorf("IsCoreRole(character, %q) = false, want true", role)
		}
	}

	places := []string{
		"메인무대", "특별한 장소", "일상 거점", "위험 구역", "금지된 장소", "기억의 장소",
		"Main stage", "Special place", "Everyday base", "Danger zone", "Forbidden place", "Place of memory",
		"メイン舞台", "特別な場所", "日常の拠点", "危険区域", "禁じられた場所", "記憶の場所",
	}
	for _, role := range places {
		if !IsCoreRole(KindPlace, role) {
			t.Errorf("IsCoreRole(place, %q) = false, want true", role)
		}
	}
}

func TestIsCoreRole_staysNarrow(t *testing.T) {
	// Core is a skeleton, not a synonym for "has a role". A walk-on with a
	// role the writer typed freehand must not push the protagonist out of the
	// limited context window.
	for _, role := range []string{"", "  ", "행인", "Barista", "通行人"} {
		if IsCoreRole(KindCharacter, role) {
			t.Errorf("IsCoreRole(character, %q) = true, want false", role)
		}
	}
	// Roles are per-kind: a place role is not a character role.
	if IsCoreRole(KindCharacter, "Main stage") {
		t.Error("a place role counted as a core character role")
	}
	// Items and concepts have no core set at all.
	if IsCoreRole(KindItem, "Protagonist") || IsCoreRole(KindConcept, "주인공") {
		t.Error("a kind with no core set returned true")
	}
}
