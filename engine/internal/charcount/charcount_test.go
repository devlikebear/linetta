package charcount

import "testing"

func TestTextCountsUnderEachBasis(t *testing.T) {
	cases := []struct {
		in       string
		with, wo int
	}{
		{"", 0, 0},
		{"안녕 하세요", 6, 5},
		{" a\tb\nc　d ", 9, 4},
		{"🙂 x", 3, 2},
	}
	for _, c := range cases {
		if got := Text(c.in, WithSpaces); got != c.with {
			t.Errorf("Text(%q, with) = %d, want %d", c.in, got, c.with)
		}
		if got := Text(c.in, WithoutSpaces); got != c.wo {
			t.Errorf("Text(%q, without) = %d, want %d", c.in, got, c.wo)
		}
	}
}

func TestDocCountsTextNodesUnderBasis(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"가 나"}]},{"type":"paragraph","content":[{"type":"text","text":"다 라 마"}]}]}`)
	if got := Doc(doc, WithSpaces); got != 8 {
		t.Errorf("with spaces = %d, want 8", got)
	}
	if got := Doc(doc, WithoutSpaces); got != 5 {
		t.Errorf("without spaces = %d, want 5", got)
	}
	if got := Doc([]byte("not json"), WithoutSpaces); got != 0 {
		t.Errorf("malformed = %d, want 0", got)
	}
}

func TestUnknownBasisCountsWithSpaces(t *testing.T) {
	if Normalize("") != WithSpaces || Normalize("bogus") != WithSpaces {
		t.Fatal("an unknown basis should count with spaces")
	}
	if Valid("") || !Valid(WithoutSpaces) {
		t.Fatal("Valid accepts exactly the two bases")
	}
}
