package vault

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/style"
)

func TestWritersStyleTravelsBetweenDevices(t *testing.T) {
	site := newFakeSite(t)
	ctx := context.Background()
	a := newDevice(t, site)
	key := a.makeKey(t)
	repoA := style.NewRepo(a.st)
	if _, err := repoA.SetWriterRules(ctx, 100, style.WriterRules{AvoidPhrases: []string{"갑자기"}, MaxSentenceChars: 70}); err != nil {
		t.Fatal(err)
	}
	if _, err := repoA.SetWriterNotes(ctx, 100, "짧은 문장으로 쓴다."); err != nil {
		t.Fatal(err)
	}
	a.syncLibrary(t)

	b := newDevice(t, site)
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}
	repoB := style.NewRepo(b.st)
	rules, err := repoB.GetWriterRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rules.AvoidPhrases, []string{"갑자기"}) || rules.MaxSentenceChars != 70 {
		t.Errorf("rules = %+v", rules)
	}
	if notes, err := repoB.GetWriterNotes(ctx); err != nil || notes != "짧은 문장으로 쓴다." {
		t.Errorf("notes = %q, %v", notes, err)
	}

	// And back.
	if _, err := repoB.SetWriterRules(ctx, 200, style.WriterRules{AvoidPhrases: []string{"갑자기", "문득"}}); err != nil {
		t.Fatal(err)
	}
	b.syncLibrary(t)
	a.syncLibrary(t)
	if rules, err := repoA.GetWriterRules(ctx); err != nil || !slices.Equal(rules.AvoidPhrases, []string{"갑자기", "문득"}) || rules.MaxSentenceChars != 0 {
		t.Errorf("rules = %+v, %v", rules, err)
	}
}

func TestRefusedStyleShowsInTheOverview(t *testing.T) {
	site := newFakeSite(t)
	ctx := context.Background()
	a := newDevice(t, site)
	key := a.makeKey(t)
	// A row over the limit, as a hand edit or a lowered limit leaves it.
	exec(t, a.db, `INSERT INTO writer_style (id, notes, updated_at) VALUES (1, ?, 100)`,
		strings.Repeat("가", style.MaxWriterNotesRunes+1))
	a.syncLibrary(t)

	b := newDevice(t, site)
	if _, err := style.NewRepo(b.st).SetWriterNotes(ctx, 50, "이 기기의 노트"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.svc.Setup(ctx, SetupInput{Consented: true, RecoveryKey: key}); err != nil {
		t.Fatal(err)
	}
	if notes, err := style.NewRepo(b.st).GetWriterNotes(ctx); err != nil || notes != "이 기기의 노트" {
		t.Errorf("notes = %q, %v", notes, err)
	}
	overview, err := b.svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.LibraryWarning != LibraryWarningStyleRefused {
		t.Fatalf("warning = %q", overview.LibraryWarning)
	}
}
