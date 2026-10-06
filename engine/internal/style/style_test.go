package style

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func scene(paragraphs ...string) []Scene {
	return []Scene{{NodeID: "n1", Label: "씬 1", Paragraphs: paragraphs}}
}

// The #162 contract: a phrase the writer listed is found exactly where it is,
// with the rule that caught it.
func TestCheck_findsAnAvoidedPhraseWhereItIs(t *testing.T) {
	rep := Check(
		scene("첫 문단은 깨끗하다.", "그는 문을 열 수 있었다. 그것은 마치 꿈같았다.", "", "셋째도 깨끗하다."),
		Rules{AvoidPhrases: []string{"그것은 마치", "수 있었다"}},
	)
	if rep.Total != 2 || len(rep.Violations) != 2 {
		t.Fatalf("total = %d, violations = %+v", rep.Total, rep.Violations)
	}
	first, second := rep.Violations[0], rep.Violations[1]
	if first.Phrase != "수 있었다" || second.Phrase != "그것은 마치" {
		t.Errorf("reading order: got %q then %q", first.Phrase, second.Phrase)
	}
	for _, v := range rep.Violations {
		if v.Rule != RuleAvoidPhrase || v.Paragraph != 2 || v.NodeID != "n1" || v.Label != "씬 1" {
			t.Errorf("violation = %+v", v)
		}
		if !strings.Contains(v.Excerpt, v.Phrase) {
			t.Errorf("excerpt %q does not show the phrase %q", v.Excerpt, v.Phrase)
		}
	}
}

// A scene that keeps the rules produces no violations at all — a check that
// cries wolf is a check the writer turns off.
func TestCheck_cleanSceneIsClean(t *testing.T) {
	rep := Check(
		scene("그는 문을 열었다.", "밖은 조용했다. 아무도 없었다."),
		Rules{AvoidPhrases: []string{"그것은 마치", "수 있었다"}, MaxSentenceChars: 40},
	)
	if rep.Total != 0 || len(rep.Violations) != 0 || rep.Truncated {
		t.Fatalf("clean scene reported %+v", rep)
	}
	if rep.Violations == nil {
		t.Error("violations must be an empty list, not null, on the wire")
	}
}

func TestCheck_countsEveryOccurrence(t *testing.T) {
	rep := Check(scene("정말 정말 좋았다. 정말이다."), Rules{AvoidPhrases: []string{"정말"}})
	if rep.Total != 3 {
		t.Fatalf("total = %d, want 3", rep.Total)
	}
}

func TestCheck_ignoresLetterCase(t *testing.T) {
	rep := Check(scene("It was Very quiet. VERY."), Rules{AvoidPhrases: []string{"very"}})
	if rep.Total != 2 {
		t.Fatalf("total = %d, want 2: %+v", rep.Total, rep.Violations)
	}
	if got := rep.Violations[0].Excerpt; !strings.Contains(got, "Very") {
		t.Errorf("excerpt should quote the scene's own casing, got %q", got)
	}
}

func TestCheck_sentenceTooLong(t *testing.T) {
	long := "그는 아주 오랫동안 아무 말도 하지 않고 창밖만 바라보다가 문득 일어났다."
	rep := Check(scene("짧다. "+long+" 다시 짧다."), Rules{MaxSentenceChars: 20})
	if rep.Total != 1 {
		t.Fatalf("total = %d, want 1: %+v", rep.Total, rep.Violations)
	}
	v := rep.Violations[0]
	if v.Rule != RuleSentenceTooLong || v.Limit != 20 || v.Length != len([]rune(long)) || v.Paragraph != 1 {
		t.Errorf("violation = %+v", v)
	}
}

func TestCheck_noRulesNoViolations(t *testing.T) {
	rep := Check(scene("무엇이든 쓴다. 그것은 마치 꿈같았다."), Rules{})
	if rep.Total != 0 || rep.ScenesChecked != 1 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestCheck_capsTheReportAndSaysSo(t *testing.T) {
	rep := Check(scene(strings.Repeat("정말 ", MaxViolations+5)), Rules{AvoidPhrases: []string{"정말"}})
	if rep.Total != MaxViolations+5 || len(rep.Violations) != MaxViolations || !rep.Truncated {
		t.Fatalf("total = %d, listed = %d, truncated = %v", rep.Total, len(rep.Violations), rep.Truncated)
	}
}

func TestCheck_isDeterministic(t *testing.T) {
	scenes := scene("그는 문을 열 수 있었다. 그것은 마치 꿈같았다.", "정말 정말.")
	rules := Rules{AvoidPhrases: []string{"정말", "그것은 마치", "수 있었다"}, MaxSentenceChars: 5}
	if a, b := Check(scenes, rules), Check(scenes, rules); !reflect.DeepEqual(a, b) {
		t.Errorf("two runs differ:\n%+v\n%+v", a, b)
	}
}

func TestSentences(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"하나. 둘! 셋?", []string{"하나.", "둘!", "셋?"}},
		{"원주율은 3.14이다. example.com에 있다.", []string{"원주율은 3.14이다.", "example.com에 있다."}},
		{"“가자.” 그가 말했다.", []string{"“가자.”", "그가 말했다."}},
		{"그런데… 아니다.", []string{"그런데…", "아니다."}},
		{"줄 하나\n줄 둘", []string{"줄 하나", "줄 둘"}},
		{"마침표 없는 문장", []string{"마침표 없는 문장"}},
		{"정말?! 그래.", []string{"정말?!", "그래."}},
		{"", nil},
	}
	for _, tc := range cases {
		if got := Sentences(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Sentences(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func openRepos(t *testing.T) (*Repo, *project.Repo, *node.Repo) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewRepo(st), project.NewRepo(st), node.NewRepo(st)
}

func newWork(t *testing.T, pr *project.Repo) project.Project {
	t.Helper()
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "문체 작품", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p
}

func TestRepo_rulesRoundTripAndNormalize(t *testing.T) {
	ctx := context.Background()
	repo, pr, _ := openRepos(t)
	p := newWork(t, pr)

	empty, err := repo.GetRules(ctx, p.ID)
	if err != nil || !empty.Empty() || empty.AvoidPhrases == nil {
		t.Fatalf("unsaved rules = %+v, %v", empty, err)
	}

	saved, err := repo.SetRules(ctx, 2000, Rules{
		ProjectID: p.ID, AvoidPhrases: []string{"  그것은   마치 ", "", "Very", "very", "수 있었다"}, MaxSentenceChars: 80,
	})
	if err != nil {
		t.Fatalf("SetRules: %v", err)
	}
	if want := []string{"그것은 마치", "Very", "수 있었다"}; !reflect.DeepEqual(saved.AvoidPhrases, want) {
		t.Errorf("normalized phrases = %q, want %q", saved.AvoidPhrases, want)
	}
	got, err := repo.GetRules(ctx, p.ID)
	if err != nil || !reflect.DeepEqual(got, saved) {
		t.Errorf("GetRules = %+v, %v; want %+v", got, err, saved)
	}

	// Saving again replaces, it does not append.
	if _, err := repo.SetRules(ctx, 3000, Rules{ProjectID: p.ID, AvoidPhrases: []string{"정말"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetRules(ctx, p.ID)
	if !reflect.DeepEqual(got.AvoidPhrases, []string{"정말"}) || got.MaxSentenceChars != 0 {
		t.Errorf("after replace = %+v", got)
	}
}

func TestRepo_refusesWhatItWouldHaveToCut(t *testing.T) {
	ctx := context.Background()
	repo, pr, _ := openRepos(t)
	p := newWork(t, pr)

	cases := []struct {
		name string
		in   Rules
		want error
	}{
		{"unknown work", Rules{ProjectID: "no-such-work"}, ErrProjectNotFound},
		{"phrase too long", Rules{ProjectID: p.ID, AvoidPhrases: []string{strings.Repeat("가", MaxPhraseRunes+1)}}, ErrPhraseTooLong},
		{"negative limit", Rules{ProjectID: p.ID, MaxSentenceChars: -1}, ErrSentenceLimit},
		{"limit over ceiling", Rules{ProjectID: p.ID, MaxSentenceChars: MaxSentenceLimit + 1}, ErrSentenceLimit},
	}
	for _, tc := range cases {
		if _, err := repo.SetRules(ctx, 2000, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	short := make([]string, MaxAvoidPhrases+1)
	for i := range short {
		short[i] = "표" + string(rune('가'+i))
	}
	if _, err := repo.SetRules(ctx, 2000, Rules{ProjectID: p.ID, AvoidPhrases: short}); !errors.Is(err, ErrTooManyPhrases) {
		t.Errorf("101 phrases: err = %v, want %v", err, ErrTooManyPhrases)
	}
}

// Deleting a work takes its rules with it.
func TestRepo_rulesAreDeletedWithTheWork(t *testing.T) {
	ctx := context.Background()
	repo, pr, _ := openRepos(t)
	p := newWork(t, pr)
	if _, err := repo.SetRules(ctx, 2000, Rules{ProjectID: p.ID, AvoidPhrases: []string{"정말"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.s.DB().ExecContext(ctx, "DELETE FROM projects WHERE id = ?", p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := repo.s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM project_style_rules").Scan(&n); err != nil || n != 0 {
		t.Errorf("rules rows after delete = %d, %v", n, err)
	}
}

func TestScenesUnder_scopesAndOrder(t *testing.T) {
	ctx := context.Background()
	_, pr, nr := openRepos(t)
	p := newWork(t, pr)
	seed := *p.LastOpenedNodeID
	doc := func(paras ...string) string {
		var b strings.Builder
		b.WriteString(`{"type":"doc","content":[`)
		for i, s := range paras {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"type":"paragraph","content":[{"type":"text","text":"` + s + `"}]}`)
		}
		b.WriteString(`]}`)
		return b.String()
	}
	if err := nr.UpdateContent(ctx, seed, doc("프롤로그 하나.", "프롤로그 둘."), 1000); err != nil {
		t.Fatal(err)
	}
	chapter, err := nr.CreateSibling(ctx, seed, node.KindContainer, "1장", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := nr.CreateChild(ctx, chapter.ID, node.KindLeaf, "씬 A", "", 1000)
	b, _ := nr.CreateChild(ctx, chapter.ID, node.KindLeaf, "씬 B", "", 1000)
	if err := nr.UpdateContent(ctx, a.ID, doc("A 본문."), 1000); err != nil {
		t.Fatal(err)
	}
	if err := nr.UpdateContent(ctx, b.ID, doc("B 본문."), 1000); err != nil {
		t.Fatal(err)
	}
	nodes, err := nr.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}

	ids := func(scenes []Scene) []string {
		out := make([]string, 0, len(scenes))
		for _, s := range scenes {
			out = append(out, s.NodeID)
		}
		return out
	}
	if got, want := ids(ScenesUnder(nodes, "")), []string{seed, a.ID, b.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("whole work = %v, want %v", got, want)
	}
	if got, want := ids(ScenesUnder(nodes, chapter.ID)), []string{a.ID, b.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("chapter = %v, want %v", got, want)
	}
	one := ScenesUnder(nodes, seed)
	if len(one) != 1 || !reflect.DeepEqual(one[0].Paragraphs, []string{"프롤로그 하나.", "프롤로그 둘."}) {
		t.Errorf("single scene = %+v", one)
	}
}
