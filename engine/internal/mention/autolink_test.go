package mention

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

func docWithText(text string) []byte {
	return []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`)
}

func testEntities() []entity.Entity {
	return []entity.Entity{
		{ID: "e-horu", Name: "호루", Kind: "character"},
		{ID: "e-naru", Name: "삼도천 나루", Kind: "place", Aliases: []string{"삼도천"}},
		{ID: "e-hwan", Name: "환", Kind: "character"}, // single rune: must never match
	}
}

func TestAutoLinkTurnsNamesIntoMentionAtoms(t *testing.T) {
	out, linked, err := AutoLinkDoc(docWithText("호루는 삼도천 나루에 서 있었다."), BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"id":"e-horu"`) || !strings.Contains(s, `"id":"e-naru"`) {
		t.Fatalf("mentions missing: %s", s)
	}
	// Korean particles attach without spaces; substring matching is the point.
	if !strings.Contains(s, "는 ") {
		t.Errorf("the particle after the name must survive as text: %s", s)
	}
	if len(linked) != 2 {
		t.Fatalf("linked = %+v, want 2 entities", linked)
	}
}

func TestAutoLinkPrefersTheLongestSurface(t *testing.T) {
	// "삼도천 나루" contains the alias "삼도천"; the full name must win.
	out, linked, err := AutoLinkDoc(docWithText("삼도천 나루의 물소리."), BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"label":"삼도천 나루"`) {
		t.Fatalf("longest surface lost to its own prefix: %s", string(out))
	}
	if len(linked) != 1 || linked[0].Count != 1 {
		t.Fatalf("linked = %+v", linked)
	}
}

func TestAutoLinkSkipsSingleRuneNamesAndExistingMentions(t *testing.T) {
	// 환 is one rune — matching it would link half the prose in a work that
	// uses the syllable at all.
	out, linked, err := AutoLinkDoc(docWithText("환한 아침이 환을 비췄다."), BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	if linked != nil {
		t.Fatalf("single-rune name linked: %+v", linked)
	}
	if strings.Contains(string(out), "mention") {
		t.Fatalf("unexpected mention: %s", string(out))
	}

	// Idempotency: a second pass over a linked doc finds nothing new, which is
	// also what makes the writer's later scene-scan a no-op.
	first, _, err := AutoLinkDoc(docWithText("호루가 웃었다."), BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	second, linked2, err := AutoLinkDoc(first, BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	if linked2 != nil {
		t.Fatalf("second pass linked again: %+v", linked2)
	}
	if string(second) != string(first) {
		t.Fatalf("second pass changed the doc:\n%s\n%s", first, second)
	}
}

func TestAutoLinkConsumesTheAtPrefixAndCountsRepeats(t *testing.T) {
	out, linked, err := AutoLinkDoc(docWithText("@호루가 말했다. 호루는 떠났다."), BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "@") {
		t.Errorf("the at-sign must be consumed with the name: %s", s)
	}
	if len(linked) != 1 || linked[0].Count != 2 {
		t.Fatalf("linked = %+v, want 호루 twice", linked)
	}
}

func TestAutoLinkPreservesMarksOnSurroundingText(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[` +
		`{"type":"text","marks":[{"type":"bold"}],"text":"호루는 강했다."}]}]}`)
	out, _, err := AutoLinkDoc(doc, BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"bold"`) {
		t.Errorf("marks on the split text were dropped: %s", s)
	}
}

// The mention resyncer derives rows from atoms; the atoms AutoLinkDoc emits
// must be the exact shape Collect understands, or linking would be invisible.
func TestAutoLinkAtomsAreCollectable(t *testing.T) {
	out, _, err := AutoLinkDoc(docWithText("호루는 삼도천 나루로 갔다."), BuildCandidates(testEntities()))
	if err != nil {
		t.Fatal(err)
	}
	found := Collect(out)
	if len(found) != 2 {
		t.Fatalf("Collect found %d mentions, want 2: %+v", len(found), found)
	}
	if found[0].EntityID != "e-horu" {
		t.Errorf("first mention = %+v", found[0])
	}
}
