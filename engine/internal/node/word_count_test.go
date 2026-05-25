package node

import (
	"encoding/json"
	"testing"
)

func TestCountChars_emptyDoc(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	if got := CountChars(doc); got != 0 {
		t.Errorf("CountChars(empty) = %d, want 0", got)
	}
}

func TestCountChars_singleParagraph(t *testing.T) {
	// "안녕 세계" is 5 chars including the space.
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"안녕 세계"}]}]}`)
	if got := CountChars(doc); got != 5 {
		t.Errorf("CountChars = %d, want 5", got)
	}
}

func TestCountChars_multipleNodes(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"파도 소리"}]},
		{"type":"paragraph","content":[{"type":"text","text":"가 멀어졌다."}]}
	]}`)
	// "파도 소리" = 5 chars, "가 멀어졌다." = 7 chars; total 12.
	if got := CountChars(doc); got != 12 {
		t.Errorf("CountChars = %d, want 12", got)
	}
}

func TestCountChars_marksAreIgnored(t *testing.T) {
	// Bold mark doesn't add chars.
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"text","text":"굵게","marks":[{"type":"bold"}]},
		{"type":"text","text":" 일반"}
	]}]}`)
	if got := CountChars(doc); got != 5 {
		t.Errorf("CountChars = %d, want 5", got)
	}
}

func TestCountChars_malformed_returnsZero(t *testing.T) {
	// Garbage input must not panic.
	cases := [][]byte{
		[]byte(`not json`),
		[]byte(`null`),
		[]byte(`{}`),
		nil,
	}
	for _, c := range cases {
		_ = json.RawMessage(c) // just to keep the import used
		if got := CountChars(c); got != 0 {
			t.Errorf("CountChars(%q) = %d, want 0", string(c), got)
		}
	}
}
