package summarizer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newFixture(t *testing.T) (*store.Store, *node.Repo, project.Project) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	return s, node.NewRepo(s), p
}

func longDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", msg)
}

func TestSummarizer_writesSummaryAndMatchesVersion(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(strings.Repeat("가나다라마", 200)), 1100)

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary != ""
	}, "summary lands")

	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	// The version stamp is what makes the summary cacheable and what stops the
	// worker from overwriting a summary an agent wrote.
	if n.SummaryForVersion != n.ContentVersion {
		t.Errorf("versions: summary_for=%d content=%d", n.SummaryForVersion, n.ContentVersion)
	}
}

func TestSummarizer_skipsWhenAlreadyFresh(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(strings.Repeat("가", 200)), 1100)
	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	_ = nodes.SetSummary(ctx, n.ID, "이미 요약됨.", n.ContentVersion)

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	time.Sleep(100 * time.Millisecond)

	got, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	if got.Summary != "이미 요약됨." {
		t.Errorf("fresh summary was replaced: %q", got.Summary)
	}
}

func TestSummarizer_shortContent_writesPlaintextWhole(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc("짧다."), 1100)

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary != ""
	}, "short summary lands")

	// Below the cut threshold the scene is its own summary, uncut.
	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	if n.Summary != "짧다." {
		t.Errorf("summary = %q, want the scene text verbatim", n.Summary)
	}
}

func TestSummarizer_reRunsAfterContentChange(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(strings.Repeat("가", 200)), 1100)

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return strings.HasPrefix(n.Summary, "가")
	}, "first summary")

	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(strings.Repeat("나", 200)), 1200)
	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return strings.HasPrefix(n.Summary, "나")
	}, "summary follows the new text")
}

func TestSummarizer_enqueueIsNonBlocking(t *testing.T) {
	_, nodes, _ := newFixture(t)
	ctx := context.Background()
	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10_000; i++ {
			sum.Enqueue("any-id")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Enqueue blocked under flood")
	}
}

func TestSummarizer_containerRollupBuildsDepth2Tree(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()

	// 부 → 장 → 씬 tree (with 2 scenes).
	part, _ := nodes.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", 1100)
	chap, _ := nodes.CreateChild(ctx, part.ID, "container", "1장", "", 1110)
	scene1, _ := nodes.CreateChild(ctx, chap.ID, "leaf", "씬 1", "", 1120)
	scene2, _ := nodes.CreateChild(ctx, chap.ID, "leaf", "씬 2", "", 1130)

	_ = nodes.UpdateContent(ctx, scene1.ID, longDoc(strings.Repeat("가나다라마", 200)), 1200)
	_ = nodes.UpdateContent(ctx, scene2.ID, longDoc(strings.Repeat("바사아자차", 200)), 1210)

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(part.ID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, part.ID)
		return n.Summary != "" && n.SummaryForVersion == n.ContentVersion
	}, "part summary lands")

	// A chapter is what its scenes are: the rollup has to name both children.
	gotChap, _ := nodes.Get(ctx, chap.ID)
	if gotChap.SummaryForVersion != gotChap.ContentVersion {
		t.Errorf("chap.summary not fresh: %+v", gotChap)
	}
	if !strings.Contains(gotChap.Summary, "씬 1") || !strings.Contains(gotChap.Summary, "씬 2") {
		t.Errorf("chapter rollup missing a scene: %q", gotChap.Summary)
	}
	gotPart, _ := nodes.Get(ctx, part.ID)
	if !strings.Contains(gotPart.Summary, "1장") {
		t.Errorf("part rollup missing its chapter: %q", gotPart.Summary)
	}
}

func TestSummarizer_containerDepthCap_stopsBeyondDepth6(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()

	// A chain of 7 containers, then a leaf: the leaf sits at depth 7 and must
	// not be reached. The cap exists so a cyclic parent_id graph cannot spin
	// the worker forever.
	parent, _ := nodes.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "C0", "", 1100)
	root := parent
	for i := 1; i < 7; i++ {
		c, _ := nodes.CreateChild(ctx, parent.ID, "container", fmt.Sprintf("C%d", i), "", int64(1100+i))
		parent = c
	}
	leaf, _ := nodes.CreateChild(ctx, parent.ID, "leaf", "씬", "", 1200)
	_ = nodes.UpdateContent(ctx, leaf.ID, longDoc(strings.Repeat("가나다라마", 200)), 1300)

	sum := New(nodes)
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(root.ID)
	time.Sleep(300 * time.Millisecond)

	got, _ := nodes.Get(ctx, leaf.ID)
	if got.Summary != "" {
		t.Errorf("leaf beyond the depth cap was summarized: %q", got.Summary)
	}
}
