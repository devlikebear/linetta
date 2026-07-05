package node

import "testing"

func TestDisplayLabel(t *testing.T) {
	cases := []struct {
		label, lang, want string
	}{
		{"1권", "ja", "第1巻"},
		{"1화", "ja", "第1話"},
		{"씬 1", "ja", "シーン 1"},
		{"1장", "ja", "第1章"},
		{"1부", "ja", "第1部"},
		{"1권", "en", "Arc 1"},
		{"1화", "en", "Episode 1"},
		{"씬 3", "en", "Scene 3"},
		{"2장", "en", "Chapter 2"},
		{"2부", "en", "Part 2"},
		{"1권", "ko", "1권"},
		{"1권", "", "1권"},
		// reverse direction: stored English label shown in Korean
		{"Scene 2", "ko", "씬 2"},
		{"Episode 4", "ja", "第4話"},
		// unrecognized labels pass through
		{"프롤로그", "en", "프롤로그"},
		{"雪の花が降る夜", "en", "雪の花が降る夜"},
	}
	for _, c := range cases {
		if got := DisplayLabel(c.label, c.lang); got != c.want {
			t.Errorf("DisplayLabel(%q, %q) = %q, want %q", c.label, c.lang, got, c.want)
		}
	}
}

func TestDisplayBreadcrumb(t *testing.T) {
	if got := DisplayBreadcrumb("1권 / 1화 / 씬 1", "ja"); got != "第1巻 / 第1話 / シーン 1" {
		t.Errorf("breadcrumb ja = %q", got)
	}
	if got := DisplayBreadcrumb("1부 / 1장 / 씬 2", "en"); got != "Part 1 / Chapter 1 / Scene 2" {
		t.Errorf("breadcrumb en = %q", got)
	}
}
