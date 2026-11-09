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

func TestValidEpisodeRange(t *testing.T) {
	for _, tc := range []struct {
		min, target, max int
		want             bool
	}{
		{0, 5000, 0, true},
		{5000, 5000, 0, true},
		{5000, 5300, 0, true},
		{4500, 5000, 5500, true},
		{0, 5500, 5500, true},
		{5000, 4999, 0, false},
		{0, 5501, 5500, false},
		{6000, 6000, 5000, false},
		{0, 0, 0, false},
		{-1, 5000, 0, false},
	} {
		if got := ValidEpisodeRange(tc.min, tc.target, tc.max); got != tc.want {
			t.Errorf("ValidEpisodeRange(%d, %d, %d) = %v, want %v", tc.min, tc.target, tc.max, got, tc.want)
		}
	}
}

// The cases episodeLength.test.ts checks, so the two implementations agree.
func TestJudgeEpisode(t *testing.T) {
	for _, tc := range []struct {
		chars, min, max int
		want            EpisodeLength
	}{
		{3925, 0, 0, EpisodeLength{State: LengthProgress}},
		{5000, 0, 0, EpisodeLength{State: LengthProgress}},
		{3925, 5000, 0, EpisodeLength{State: LengthShort, Delta: -1075}},
		{0, 5000, 0, EpisodeLength{State: LengthShort, Delta: -5000}},
		{5000, 5000, 0, EpisodeLength{State: LengthOK}},
		{5500, 4500, 5500, EpisodeLength{State: LengthOK}},
		{5820, 4500, 5500, EpisodeLength{State: LengthLong, Delta: 320}},
		{5820, 0, 5500, EpisodeLength{State: LengthLong, Delta: 320}},
	} {
		if got := JudgeEpisode(tc.chars, tc.min, tc.max); got != tc.want {
			t.Errorf("JudgeEpisode(%d, %d, %d) = %+v, want %+v", tc.chars, tc.min, tc.max, got, tc.want)
		}
	}
}

func TestGroup(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 5300: "5,300", 1234567: "1,234,567", -1075: "-1,075"} {
		if got := Group(n); got != want {
			t.Errorf("Group(%d) = %q, want %q", n, got, want)
		}
	}
}
