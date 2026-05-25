package mention

import "testing"

func TestCollect_emptyDoc(t *testing.T) {
	if got := Collect([]byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)); len(got) != 0 {
		t.Errorf("empty doc → %d mentions, want 0", len(got))
	}
}

func TestCollect_findsMentionAtoms(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"파도 소리. "},
			{"type":"mention","attrs":{"id":"e1","label":"해진"}},
			{"type":"text","text":"가 모래를 밟았다. "},
			{"type":"mention","attrs":{"id":"e2","label":"윤서"}}
		]}
	]}`)
	got := Collect(doc)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].EntityID != "e1" || got[0].Surface != "해진" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].EntityID != "e2" || got[1].Surface != "윤서" {
		t.Errorf("second = %+v", got[1])
	}
	if !(got[0].Position < got[1].Position) {
		t.Errorf("positions not monotonically increasing: %d, %d", got[0].Position, got[1].Position)
	}
}

func TestCollect_ignoresMalformedAtoms(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[
		{"type":"paragraph","content":[
			{"type":"mention","attrs":{"label":"X"}},
			{"type":"mention","attrs":{"id":"","label":""}}
		]}
	]}`)
	if got := Collect(doc); len(got) != 0 {
		t.Errorf("malformed → got %d, want 0", len(got))
	}
}

func TestCollect_malformedJSON_returnsEmpty(t *testing.T) {
	if got := Collect([]byte(`not json`)); len(got) != 0 {
		t.Errorf("bad json → %d mentions", len(got))
	}
}
