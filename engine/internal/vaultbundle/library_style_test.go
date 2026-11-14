package vaultbundle

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/style"
)

func (l lib) styleRepo() *style.Repo { return style.NewRepo(l.st) }

func (l lib) setRules(t *testing.T, when int64, phrases []string, limit int) {
	t.Helper()
	if _, err := l.styleRepo().SetWriterRules(context.Background(), when, style.WriterRules{AvoidPhrases: phrases, MaxSentenceChars: limit}); err != nil {
		t.Fatal(err)
	}
}

func (l lib) setNotes(t *testing.T, when int64, notes string) {
	t.Helper()
	if _, err := l.styleRepo().SetWriterNotes(context.Background(), when, notes); err != nil {
		t.Fatal(err)
	}
}

func (l lib) rules(t *testing.T) style.WriterRules {
	t.Helper()
	rules, err := l.styleRepo().GetWriterRules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func (l lib) notes(t *testing.T) string {
	t.Helper()
	notes, err := l.styleRepo().GetWriterNotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return notes
}

func (l lib) draft(t *testing.T) (style.Draft, bool) {
	t.Helper()
	draft, ok, err := l.styleRepo().GetWriterDraft(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return draft, ok
}

func TestLibraryCarriesTheWritersStyle(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.setRules(t, 100, []string{"갑자기", "그리고 나서"}, 80)
	a.setNotes(t, 200, "짧은 문장으로 리듬을 만든다.")
	if _, err := a.styleRepo().SaveWriterDraft(context.Background(), 300, "현재형으로 쓴다.", "companion"); err != nil {
		t.Fatal(err)
	}

	bundle := a.export(t, LibrarySettings{})
	if bundle.Meta.Format != LibraryFormat {
		t.Errorf("format = %d", bundle.Meta.Format)
	}
	out := b.apply(t, bundle, nil, LibrarySettings{})

	want := []string{libraryStyleDraftPart, libraryStyleNotesPart, libraryStyleRulesPart}
	if got := slices.Sorted(slices.Values(out.Taken)); !slices.Equal(got, want) {
		t.Fatalf("taken %v", out.Taken)
	}
	rules := b.rules(t)
	if !slices.Equal(rules.AvoidPhrases, []string{"갑자기", "그리고 나서"}) || rules.MaxSentenceChars != 80 {
		t.Errorf("rules = %+v", rules)
	}
	if got := b.notes(t); got != "짧은 문장으로 리듬을 만든다." {
		t.Errorf("notes = %q", got)
	}
	draft, ok := b.draft(t)
	if !ok || draft.Body != "현재형으로 쓴다." || draft.Author != "companion" || draft.CreatedAt != 300 {
		t.Errorf("draft = %+v, %v", draft, ok)
	}
	if a.export(t, LibrarySettings{}).ContentHash() != b.export(t, LibrarySettings{}).ContentHash() {
		t.Error("two libraries in step hash differently")
	}
}

func TestLibraryWithoutStyleKeepsTheOldFormat(t *testing.T) {
	a := newLib(t)
	a.profile(t, "프로필", 1)
	if got := a.export(t, LibrarySettings{}).Meta.Format; got != libraryFormatNoStyle {
		t.Errorf("format = %d", got)
	}
	a.setNotes(t, 2, "노트")
	if got := a.export(t, LibrarySettings{Omit: []string{CategoryWriterProfile}}).Meta.Format; got != libraryFormatNoStyle {
		t.Errorf("format with the style left out = %d", got)
	}
}

// Rules changed on one device and notes on the other are separate parts: both
// edits stand.
func TestLibraryStyleRulesAndNotesMergeApart(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.setRules(t, 100, []string{"갑자기"}, 0)
	a.setNotes(t, 100, "옛 노트")
	b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})
	base := a.export(t, LibrarySettings{}).PartHashes()

	a.setRules(t, 300, []string{"갑자기", "문득"}, 0)
	b.setNotes(t, 400, "새 노트")
	b.apply(t, a.export(t, LibrarySettings{}), base, LibrarySettings{})

	if got := b.rules(t).AvoidPhrases; !slices.Equal(got, []string{"갑자기", "문득"}) {
		t.Errorf("rules = %v", got)
	}
	if got := b.notes(t); got != "새 노트" {
		t.Errorf("notes = %q", got)
	}
	// Taking the older rules does not wind back the row's newer time.
	if got := b.rules(t).UpdatedAt; got != 400 {
		t.Errorf("updated_at = %d", got)
	}
}

func TestLibraryNewerStyleWinsAConflict(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.setNotes(t, 100, "거기 노트")
	b.setNotes(t, 200, "여기 노트")
	b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})
	if got := b.notes(t); got != "여기 노트" {
		t.Fatalf("notes = %q", got)
	}
	a.setNotes(t, 300, "더 새 노트")
	b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})
	if got := b.notes(t); got != "더 새 노트" {
		t.Errorf("notes = %q", got)
	}
}

func TestLibraryStyleClearingTravels(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.setRules(t, 100, []string{"갑자기"}, 60)
	a.setNotes(t, 100, "노트")
	if _, err := a.styleRepo().SaveWriterDraft(context.Background(), 100, "초안", "companion"); err != nil {
		t.Fatal(err)
	}
	b.apply(t, a.export(t, LibrarySettings{}), nil, LibrarySettings{})
	base := a.export(t, LibrarySettings{}).PartHashes()

	// The writer approves the draft over the notes and clears the rules.
	if _, err := a.styleRepo().ApproveWriterDraft(context.Background(), 200, "초안", style.ApproveReplace); err != nil {
		t.Fatal(err)
	}
	a.setRules(t, 200, nil, 0)
	out := b.apply(t, a.export(t, LibrarySettings{}), base, LibrarySettings{})

	if got := slices.Sorted(slices.Values(out.Removed)); !slices.Equal(got, []string{libraryStyleDraftPart, libraryStyleRulesPart}) {
		t.Errorf("removed %v", out.Removed)
	}
	if rules := b.rules(t); !rules.Empty() {
		t.Errorf("rules = %+v", rules)
	}
	if got := b.notes(t); got != "초안" {
		t.Errorf("notes = %q", got)
	}
	if _, ok := b.draft(t); ok {
		t.Error("the approved draft is still pending")
	}
}

func TestLibraryStyleStaysOutWithTheWriterProfile(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.setRules(t, 100, []string{"거기"}, 0)
	a.setNotes(t, 100, "거기 노트")
	b.setRules(t, 50, []string{"여기"}, 0)
	b.setNotes(t, 50, "여기 노트")
	selection := LibrarySettings{Omit: []string{CategoryWriterProfile}}

	bundle := a.export(t, selection)
	for name := range bundle.Parts {
		if isStylePart(name) {
			t.Fatalf("part %s travels with the writer profile left out", name)
		}
	}
	for _, apply := range []struct {
		remote *LibraryBundle
		local  LibrarySettings
	}{{bundle, LibrarySettings{}}, {a.export(t, LibrarySettings{}), selection}} {
		out := b.apply(t, apply.remote, nil, apply.local)
		if len(out.Taken)+len(out.Removed) != 0 {
			t.Fatalf("touched %+v", out)
		}
		if got := b.rules(t).AvoidPhrases; !slices.Equal(got, []string{"여기"}) {
			t.Errorf("rules = %v", got)
		}
		if got := b.notes(t); got != "여기 노트" {
			t.Errorf("notes = %q", got)
		}
	}
}

func TestLibraryRefusesStyleOverTheLimits(t *testing.T) {
	a, b := newLib(t), newLib(t)
	a.setNotes(t, 100, "짧다")
	a.setRules(t, 100, []string{"갑자기"}, 0)
	bundle := a.export(t, LibrarySettings{})
	bundle.Parts[libraryStyleNotesPart] = []byte(strings.Repeat("가", style.MaxWriterNotesRunes+1))
	bundle.Parts[libraryStyleRulesPart] = []byte(`{"avoid_phrases":[],"max_sentence_chars":99999}`)
	bundle.seal()
	b.setNotes(t, 50, "여기 노트")
	b.setRules(t, 50, []string{"여기"}, 0)

	out := b.apply(t, bundle, nil, LibrarySettings{})

	if !out.StyleRefused {
		t.Fatal("not refused")
	}
	if got := b.notes(t); got != "여기 노트" {
		t.Errorf("notes = %q", got)
	}
	if got := b.rules(t); !slices.Equal(got.AvoidPhrases, []string{"여기"}) || got.MaxSentenceChars != 0 {
		t.Errorf("rules = %+v", got)
	}
}

func TestSizesCountTheStyleAsTheWriterProfile(t *testing.T) {
	l := newLib(t)
	l.setNotes(t, 1, "노트")
	sizes, err := Sizes(context.Background(), l.db, l.home, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if sizes[CategoryWriterProfile] != int64(len("노트")) {
		t.Errorf("sizes = %v", sizes)
	}
}
