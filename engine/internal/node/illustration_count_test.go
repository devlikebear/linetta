package node

import "testing"

// A picture adds nothing to a scene's length: only text is counted.
func TestCountCharsIgnoresIllustrations(t *testing.T) {
	without := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]}]}`
	with := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]},` +
		`{"type":"illustration","attrs":{"id":"pic-1","caption":"아주 긴 캡션이 있는 그림"}}]}`
	if a, b := CountChars([]byte(without)), CountChars([]byte(with)); a != b {
		t.Fatalf("count without = %d, with a picture = %d", a, b)
	}
}
