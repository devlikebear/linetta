package ftsquery

import "testing"

func TestTermsDropsOperatorsAndKeepsPunctuationOnlyQueries(t *testing.T) {
	if got := Terms(`"곰배" AND 표두`); len(got) != 2 || got[0] != "곰배" || got[1] != "표두" {
		t.Errorf("Terms = %q", got)
	}
	if got := Terms("***"); len(got) != 1 || got[0] != "***" {
		t.Errorf("a query of only punctuation must find itself, got %q", got)
	}
}

func TestTrigramMatchRefusesAShortTermRatherThanDroppingIt(t *testing.T) {
	if _, ok := TrigramMatch([]string{"무형검", "서린"}); ok {
		t.Error("a two-character term must send the whole query to LIKE")
	}
	got, ok := TrigramMatch([]string{"무형검", `그"것은`})
	if !ok || got != `"무형검" AND "그""것은"` {
		t.Errorf("TrigramMatch = %q, %v", got, ok)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := EscapeLike(`100%_a\`); got != `100\%\_a\\` {
		t.Errorf("EscapeLike = %q", got)
	}
}

func TestLikeSnippetCountsRunesAndPrefersTheFirstNeedle(t *testing.T) {
	got := LikeSnippet("가나다라마바사아자차", []string{"없음", "바사"}, 2)
	if got != "...라마바사아자..." {
		t.Errorf("LikeSnippet = %q", got)
	}
}
