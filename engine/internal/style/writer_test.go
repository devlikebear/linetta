package style

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// #179's contract, on the merge a check runs against.

func TestMerge_aWritersPhraseIsCaughtAndSaysWhoseItIs(t *testing.T) {
	rules := Merge(Rules{AvoidPhrases: []string{"그것은 마치"}}, WriterRules{AvoidPhrases: []string{"하는 것이었다"}})
	rep := Check(scene("그것은 마치 꿈같았다. 그가 하는 것이었다."), rules)
	sources := map[string]string{}
	for _, v := range rep.Violations {
		if v.Rule == RuleAvoidPhrase {
			sources[v.Phrase] = v.Source
		}
	}
	if sources["하는 것이었다"] != SourceWriter || sources["그것은 마치"] != SourceWork {
		t.Fatalf("sources = %v", sources)
	}
}

func TestMerge_aWorkThatOptsOutChecksOnlyItsOwn(t *testing.T) {
	rules := Merge(Rules{IgnoreWriterStyle: true}, WriterRules{AvoidPhrases: []string{"하는 것이었다"}, MaxSentenceChars: 10})
	rep := Check(scene("그가 하는 것이었다. 아주 길고 긴 문장이 여기에 있다."), rules)
	for _, v := range rep.Violations {
		if v.Rule == RuleAvoidPhrase || v.Rule == RuleSentenceTooLong {
			t.Fatalf("an opted-out work broke the writer's rule: %+v", v)
		}
	}
}

func TestMerge_sentenceLimitIsTheWorksOwnElseTheWriters(t *testing.T) {
	sentence := strings.Repeat("가", 99) + "."
	writer := WriterRules{MaxSentenceChars: 80}

	own := Merge(Rules{MaxSentenceChars: 120, OwnSentenceLimit: true}, writer)
	if rep := Check(scene(sentence), own); hasRule(rep, RuleSentenceTooLong) {
		t.Fatalf("a 100-character sentence broke the work's 120 limit: %+v", rep.Violations)
	}

	inherited := Merge(Rules{}, writer)
	rep := Check(scene(sentence), inherited)
	v, ok := firstRule(rep, RuleSentenceTooLong)
	if !ok || v.Limit != 80 || v.Source != SourceWriter {
		t.Fatalf("inherited limit: %+v", rep.Violations)
	}

	// "No limit" of the work's own is not "the writer's limit".
	none := Merge(Rules{OwnSentenceLimit: true}, writer)
	if rep := Check(scene(sentence), none); hasRule(rep, RuleSentenceTooLong) {
		t.Fatalf("a work with no limit of its own got the writer's: %+v", rep.Violations)
	}
}

func TestMerge_aPhraseOnBothListsIsTheWorksAndCountedOnce(t *testing.T) {
	rules := Merge(Rules{AvoidPhrases: []string{"Very"}}, WriterRules{AvoidPhrases: []string{"very", "quite"}})
	if strings.Join(rules.AvoidPhrases, ",") != "Very,quite" || strings.Join(rules.WriterPhrases, ",") != "quite" {
		t.Fatalf("merged = %+v", rules)
	}
	rep := Check(scene("very good"), rules)
	if rep.Total != 1 || rep.Violations[0].Source != SourceWork {
		t.Fatalf("report = %+v", rep)
	}
}

// With no writer rules, merging changes nothing a check reports.
func TestMerge_emptyWriterRulesChangeNothing(t *testing.T) {
	work := Rules{AvoidPhrases: []string{"마치"}, MaxSentenceChars: 20, OwnSentenceLimit: true}
	text := scene("마치 꿈처럼. " + strings.Repeat("나", 30) + ".")
	before, after := Check(text, work), Check(text, Merge(work, WriterRules{}))
	if before.Total != after.Total || len(before.Violations) != len(after.Violations) {
		t.Fatalf("before %+v, after %+v", before, after)
	}
}

func TestWriterRules_storeValidateAndReach(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	repo := NewRepo(st)

	if got, err := repo.GetWriterRules(ctx); err != nil || !got.Empty() || got.AvoidPhrases == nil {
		t.Fatalf("fresh writer rules = %+v, %v", got, err)
	}
	if _, err := repo.SetWriterRules(ctx, 1, WriterRules{MaxSentenceChars: MaxSentenceLimit + 1}); !errors.Is(err, ErrSentenceLimit) {
		t.Fatalf("over the limit: %v", err)
	}
	if _, err := repo.SetWriterRules(ctx, 1, WriterRules{AvoidPhrases: []string{strings.Repeat("가", MaxPhraseRunes+1)}}); !errors.Is(err, ErrPhraseTooLong) {
		t.Fatalf("long phrase: %v", err)
	}
	saved, err := repo.SetWriterRules(ctx, 2, WriterRules{AvoidPhrases: []string{" 하는  것이었다 ", "하는 것이었다", ""}, MaxSentenceChars: 80})
	if err != nil || strings.Join(saved.AvoidPhrases, "|") != "하는 것이었다" || saved.MaxSentenceChars != 80 {
		t.Fatalf("saved = %+v, %v", saved, err)
	}

	// Every work sees them at once, and an opted-out one does not.
	pr := project.NewRepo(st)
	a, _ := pr.Create(ctx, 1, project.NewInput{Title: "A", LengthTarget: "short", DefaultPOV: "first"})
	b, _ := pr.Create(ctx, 1, project.NewInput{Title: "B", LengthTarget: "short", DefaultPOV: "first"})
	if _, err := repo.SetRules(ctx, 3, Rules{ProjectID: b.ID, IgnoreWriterStyle: true}); err != nil {
		t.Fatal(err)
	}
	ea, _ := repo.EffectiveRules(ctx, a.ID)
	eb, _ := repo.EffectiveRules(ctx, b.ID)
	if strings.Join(ea.WriterPhrases, "|") != "하는 것이었다" || ea.MaxSentenceChars != 80 || ea.SentenceLimitSource != SourceWriter {
		t.Fatalf("work A = %+v", ea)
	}
	if len(eb.AvoidPhrases) != 0 || eb.MaxSentenceChars != 0 || !eb.IgnoreWriterStyle {
		t.Fatalf("opted-out work B = %+v", eb)
	}

	// The old device setting folds in without losing what is there.
	if err := repo.AdoptPhrases(ctx, 4, []string{"하는 것이었다", "그것은 마치", strings.Repeat("나", MaxPhraseRunes+1)}); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetWriterRules(ctx)
	if strings.Join(got.AvoidPhrases, "|") != "하는 것이었다|그것은 마치" || got.MaxSentenceChars != 80 {
		t.Fatalf("after adopting = %+v", got)
	}
}

func hasRule(rep Report, rule string) bool {
	_, ok := firstRule(rep, rule)
	return ok
}

func firstRule(rep Report, rule string) (Violation, bool) {
	for _, v := range rep.Violations {
		if v.Rule == rule {
			return v, true
		}
	}
	return Violation{}, false
}

func TestWriterNotesAndDraft(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	repo := NewRepo(st)

	// Notes and rules share a row without clobbering each other.
	if _, err := repo.SetWriterRules(ctx, 1, WriterRules{AvoidPhrases: []string{"마치"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetWriterNotes(ctx, 2, "  현재형.  "); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetWriterRules(ctx, 3, WriterRules{AvoidPhrases: []string{"마치", "문득"}}); err != nil {
		t.Fatal(err)
	}
	if notes, _ := repo.GetWriterNotes(ctx); notes != "현재형." {
		t.Fatalf("notes = %q", notes)
	}
	if _, err := repo.SetWriterNotes(ctx, 4, strings.Repeat("가", MaxWriterNotesRunes+1)); !errors.Is(err, ErrWriterNotesTooLong) {
		t.Fatalf("over the limit: %v", err)
	}

	// A work that opts out carries none of it into its brief.
	p, _ := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "A", LengthTarget: "short", DefaultPOV: "first"})
	if got, _ := repo.WriterNotesFor(ctx, p.ID); got != "현재형." {
		t.Fatalf("following work gets %q", got)
	}
	if _, err := repo.SetRules(ctx, 5, Rules{ProjectID: p.ID, IgnoreWriterStyle: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.WriterNotesFor(ctx, p.ID); got != "" {
		t.Fatalf("opted-out work gets %q", got)
	}

	// Drafts: nothing to approve, then append, then a refusal over the limit
	// that leaves the notes and the draft as they were.
	if _, err := repo.ApproveWriterDraft(ctx, 6, "x", ApproveAppend); !errors.Is(err, ErrNoDraft) {
		t.Fatalf("approve with no draft: %v", err)
	}
	if _, err := repo.SaveWriterDraft(ctx, 7, "짧게.", "agent"); err != nil {
		t.Fatal(err)
	}
	notes, err := repo.ApproveWriterDraft(ctx, 8, "짧게.", ApproveAppend)
	if err != nil || notes != "현재형.\n\n짧게." {
		t.Fatalf("appended = %q, %v", notes, err)
	}
	if _, pending, _ := repo.GetWriterDraft(ctx); pending {
		t.Fatal("draft left after approval")
	}
	if _, err := repo.SaveWriterDraft(ctx, 9, strings.Repeat("나", 600), "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetWriterNotes(ctx, 10, strings.Repeat("다", 600)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApproveWriterDraft(ctx, 11, strings.Repeat("나", 600), ApproveAppend); !errors.Is(err, ErrWriterNotesTooLong) {
		t.Fatalf("append over the limit: %v", err)
	}
	if _, pending, _ := repo.GetWriterDraft(ctx); !pending {
		t.Fatal("a refused approval dropped the draft")
	}
	if notes, err := repo.ApproveWriterDraft(ctx, 12, strings.Repeat("나", 600), ApproveReplace); err != nil || notes != strings.Repeat("나", 600) {
		t.Fatalf("replace = %d runes, %v", len([]rune(notes)), err)
	}
}
