package manuscript

import (
	"context"
	"errors"
	"testing"
)

type fakeSemantic struct {
	matches []SemanticMatch
	ok      bool
	err     error
}

func (f fakeSemantic) Search(context.Context, string, string, int) ([]SemanticMatch, bool, error) {
	return f.matches, f.ok, f.err
}

func TestQueryMergesWordsAndMeaning(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	nr.SetManuscriptIndexer(indexer)
	first := *p.LastOpenedNodeID
	if err := nr.UpdateContent(ctx, first, tiptapDoc("진홍빛 눈동자가 빛났다."), 2); err != nil {
		t.Fatal(err)
	}
	piano := sceneAfter(t, nr, first, "씬 2", "누나는 건반 위에 손가락을 올렸다.")
	other := sceneAfter(t, nr, piano, "씬 3", "비가 왔다.")
	searcher := NewSearcher(st.DB(), nr, indexer)

	// Meaning finds the piano scene though no word matches; the words find
	// the first scene; both make the list, the piano scene with its
	// paragraph, a snippet and a breadcrumb.
	searcher.SetSemantic(fakeSemantic{ok: true, matches: []SemanticMatch{
		{NodeID: piano, Para: 1, Text: "누나는 건반 위에 손가락을 올렸다.", Score: 0.9},
		{NodeID: other, Para: 1, Text: "비가 왔다.", Score: 0.2},
	}})
	hits, semantic, err := searcher.QueryMode(ctx, p.ID, "진홍빛", 5, ModeAuto)
	if err != nil || !semantic {
		t.Fatalf("err = %v semantic = %v", err, semantic)
	}
	if len(hits) != 3 || hits[0].NodeID != first {
		t.Fatalf("hits = %+v, want the word match first (in both lists' tops)", hits)
	}
	var found bool
	for _, h := range hits {
		if h.NodeID == piano {
			found = true
			if h.ParagraphFrom != 1 || h.Snippet == "" || h.Breadcrumb == "" {
				t.Fatalf("meaning-only hit = %+v", h)
			}
		}
	}
	if !found {
		t.Fatal("meaning-only scene missing")
	}

	// Meaning alone drops what only the words found.
	hits, _, _ = searcher.QueryMode(ctx, p.ID, "진홍빛", 5, ModeMeaning)
	if len(hits) != 2 || hits[0].NodeID != piano {
		t.Fatalf("meaning hits = %+v", hits)
	}
	// Words alone never asks.
	hits, semantic, _ = searcher.QueryMode(ctx, p.ID, "진홍빛", 5, ModeWords)
	if semantic || len(hits) != 1 {
		t.Fatalf("words hits = %+v semantic %v", hits, semantic)
	}
}

func TestQueryFallsBackToWordsWhenMeaningCannotAnswer(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	nr.SetManuscriptIndexer(indexer)
	if err := nr.UpdateContent(ctx, *p.LastOpenedNodeID, tiptapDoc("진홍빛 눈동자가 빛났다."), 2); err != nil {
		t.Fatal(err)
	}
	searcher := NewSearcher(st.DB(), nr, indexer)
	for _, sem := range []fakeSemantic{{ok: false}, {ok: true, err: errors.New("model broke")}} {
		searcher.SetSemantic(sem)
		hits, semantic, err := searcher.QueryMode(ctx, p.ID, "진홍빛", 5, ModeMeaning)
		if err != nil || semantic || len(hits) != 1 {
			t.Fatalf("%+v: hits = %+v semantic %v err %v", sem, hits, semantic, err)
		}
	}
}
