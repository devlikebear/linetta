package manuscript

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/style"
)

// paragraphsDoc builds a doc with one paragraph per entry; "" is an empty
// paragraph, which style.Paragraphs does not number.
func paragraphsDoc(paras ...string) string {
	content := make([]any, 0, len(paras))
	for _, p := range paras {
		block := map[string]any{"type": "paragraph"}
		if p != "" {
			block["content"] = []any{map[string]any{"type": "text", "text": p}}
		}
		content = append(content, block)
	}
	raw, _ := json.Marshal(map[string]any{"type": "doc", "content": content})
	return string(raw)
}

func TestChunkParagraphsGrowsToTheMinimum(t *testing.T) {
	p := strings.Repeat("가", 120)
	got := chunkParagraphs([]string{p, p, p, p, p, p})
	want := [][2]int{{1, 3}, {4, 6}}
	if len(got) != len(want) {
		t.Fatalf("chunks = %+v, want ranges %v", got, want)
	}
	for i, c := range got {
		if c.ParaFrom != want[i][0] || c.ParaTo != want[i][1] {
			t.Fatalf("chunk %d = %d-%d, want %v", i, c.ParaFrom, c.ParaTo, want[i])
		}
		if n := len(c.paragraphs()); n != c.ParaTo-c.ParaFrom+1 {
			t.Fatalf("chunk %d splits into %d paragraphs, range says %d", i, n, c.ParaTo-c.ParaFrom+1)
		}
	}
}

func TestChunkParagraphsKeepsALongParagraphAlone(t *testing.T) {
	long := strings.Repeat("나", chunkMaxRunes+50)
	short := strings.Repeat("다", 100)
	got := chunkParagraphs([]string{short, long, short})
	want := [][2]int{{1, 1}, {2, 2}, {3, 3}}
	if len(got) != len(want) {
		t.Fatalf("chunks = %+v, want ranges %v", got, want)
	}
	for i, c := range got {
		if c.ParaFrom != want[i][0] || c.ParaTo != want[i][1] {
			t.Fatalf("chunk %d = %d-%d, want %v", i, c.ParaFrom, c.ParaTo, want[i])
		}
	}
	if chunkParagraphs(nil) != nil {
		t.Fatal("no paragraphs should make no chunks")
	}
}

// The numbers a chunk carries must be the numbers every other feature uses,
// or a search result points one paragraph off on any scene with a blank line.
func TestChunksNumberParagraphsLikeStyleParagraphs(t *testing.T) {
	doc := `{"type":"doc","content":[
{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"제목"}]},
{"type":"paragraph"},
{"type":"paragraph","content":[{"type":"text","text":"첫 줄"},{"type":"hardBreak"},{"type":"text","text":"둘째 줄"}]},
{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"인용"}]}]},
{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"e1","label":"수아"}},{"type":"text","text":"가 웃었다."}]}
]}`
	want := style.Paragraphs(doc)
	if len(want) < 4 {
		t.Fatalf("fixture has %d paragraphs, want at least 4: %q", len(want), want)
	}
	got := map[int]string{}
	for _, c := range Chunks(doc) {
		for j, p := range c.paragraphs() {
			got[c.ParaFrom+j] = p
		}
	}
	if len(got) != len(want) {
		t.Fatalf("chunks cover %d paragraphs, style.Paragraphs has %d", len(got), len(want))
	}
	for i, p := range want {
		if got[i+1] != p {
			t.Fatalf("paragraph %d = %q, want %q", i+1, got[i+1], p)
		}
	}
}

func chunkRanges(t *testing.T, ctx context.Context, idx *Indexer, nodeID string) []string {
	t.Helper()
	rows, err := idx.db.QueryContext(ctx, `SELECT para_from || '-' || para_to FROM manuscript_chunks WHERE node_id = ? ORDER BY para_from`, nodeID)
	if err != nil {
		t.Fatalf("chunks: %v", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestIndexerKeepsChunksInStepWithTheScene(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	idx := NewIndexer(st.DB())
	sceneID := *p.LastOpenedNodeID
	long := strings.Repeat("라", 250)

	if err := idx.Upsert(ctx, p.ID, sceneID, paragraphsDoc(long, long, long)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got := strings.Join(chunkRanges(t, ctx, idx, sceneID), ","); got != "1-2,3-3" {
		t.Fatalf("chunks = %s, want 1-2,3-3", got)
	}
	if err := idx.Upsert(ctx, p.ID, sceneID, paragraphsDoc("짧다")); err != nil {
		t.Fatalf("Upsert again: %v", err)
	}
	if got := strings.Join(chunkRanges(t, ctx, idx, sceneID), ","); got != "1-1" {
		t.Fatalf("chunks after rewrite = %s, want 1-1", got)
	}
	if err := idx.Delete(ctx, sceneID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := chunkRanges(t, ctx, idx, sceneID); len(got) != 0 {
		t.Fatalf("chunks after delete = %v", got)
	}

	// Rebuild and RebuildAll write the committed content back and drop rows
	// whose scene is gone.
	if err := nr.UpdateContent(ctx, sceneID, paragraphsDoc(long, long, long), 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `INSERT INTO manuscript_chunks (project_id, node_id, para_from, para_to, text, text_hash) VALUES (?, 'gone', 1, 1, 'x', 'h')`, p.ID); err != nil {
		t.Fatal(err)
	}
	for name, rebuild := range map[string]func() error{
		"Rebuild":    func() error { return idx.Rebuild(ctx, p.ID) },
		"RebuildAll": func() error { return idx.RebuildAll(ctx) },
	} {
		if err := rebuild(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := strings.Join(chunkRanges(t, ctx, idx, sceneID), ","); got != "1-2,3-3" {
			t.Fatalf("%s: chunks = %s, want 1-2,3-3", name, got)
		}
		if got := chunkRanges(t, ctx, idx, "gone"); len(got) != 0 {
			t.Fatalf("%s kept an orphan chunk: %v", name, got)
		}
	}
}

func TestSearchHitNamesTheParagraph(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	idx := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, idx)
	sceneID := *p.LastOpenedNodeID
	filler := strings.Repeat("비가 내렸다. ", 30)
	// The empty paragraph is not numbered, so "진홍빛" is paragraph 4, not 5.
	doc := paragraphsDoc(filler, filler, "", filler, "수아의 눈동자는 진홍빛이었다.", filler, "민호는 은빛 열쇠를 쥐었다.", filler)
	if err := idx.Upsert(ctx, p.ID, sceneID, doc); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	for _, tc := range []struct {
		query    string
		from, to int
	}{
		{"진홍빛", 4, 4},       // trigram path
		{"수아", 4, 4},        // short-term LIKE path
		{"진홍빛이었다 수아", 4, 4}, // two words, one paragraph
		// Words split across paragraphs: the chunk holding the most of them,
		// narrowed to its paragraphs that hold any. Paragraphs 3-5 are one
		// chunk; 4 and 6 are not.
		{"진홍빛이었다 비가", 3, 5},
		{"진홍빛 은빛 열쇠", 6, 6},
		{"열쇠", 6, 6},     // a later paragraph
		{"비가 내렸다", 1, 1}, // everywhere: the earliest passage
	} {
		hits, err := searcher.Query(ctx, p.ID, tc.query, 5)
		if err != nil {
			t.Fatalf("%q: %v", tc.query, err)
		}
		if len(hits) != 1 {
			t.Fatalf("%q: hits = %+v, want 1", tc.query, hits)
		}
		if hits[0].ParagraphFrom != tc.from || hits[0].ParagraphTo != tc.to {
			t.Fatalf("%q: paragraphs %d-%d, want %d-%d", tc.query, hits[0].ParagraphFrom, hits[0].ParagraphTo, tc.from, tc.to)
		}
	}

	paras := style.Paragraphs(doc)
	if !strings.Contains(paras[4-1], "진홍빛") || !strings.Contains(paras[6-1], "열쇠") {
		t.Fatalf("fixture numbering drifted from style.Paragraphs: %q", paras)
	}
}

func TestSearchHitWithoutChunksHasNoRange(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	idx := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, idx)
	sceneID := *p.LastOpenedNodeID
	if err := idx.Upsert(ctx, p.ID, sceneID, tiptapDoc("수아의 눈동자는 진홍빛이었다.")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM manuscript_chunks`); err != nil {
		t.Fatal(err)
	}
	hits, err := searcher.Query(ctx, p.ID, "진홍빛", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits = %+v, err = %v", hits, err)
	}
	if hits[0].ParagraphFrom != 0 || hits[0].ParagraphTo != 0 {
		t.Fatalf("a hit with no chunks to place it got %d-%d", hits[0].ParagraphFrom, hits[0].ParagraphTo)
	}
}

// Startup rebuilds the whole library, so a scene whose chunks are already
// right is left alone, and only a scene that changed is rewritten.
func TestRebuildAllRewritesOnlyTheScenesThatChanged(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	idx := NewIndexer(st.DB())
	first := *p.LastOpenedNodeID
	long := strings.Repeat("마", 250)
	if err := nr.UpdateContent(ctx, first, paragraphsDoc(long, long), 2000); err != nil {
		t.Fatal(err)
	}
	second := sceneAfter(t, nr, first, "씬 2", "둘째 장면")
	if err := idx.RebuildAll(ctx); err != nil {
		t.Fatal(err)
	}
	rowids := func(nodeID string) string {
		var out string
		if err := st.DB().QueryRowContext(ctx, `SELECT group_concat(rowid) FROM (SELECT rowid FROM manuscript_chunks WHERE node_id = ? ORDER BY para_from)`, nodeID).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	keep := rowids(first)

	// The repo has no indexer here, so the index only learns of this edit
	// from the rebuild.
	if err := nr.UpdateContent(ctx, second, paragraphsDoc("바뀐 장면", "새 문단"), 5000); err != nil {
		t.Fatal(err)
	}
	if err := idx.RebuildAll(ctx); err != nil {
		t.Fatal(err)
	}
	if got := rowids(first); got != keep {
		t.Fatalf("unchanged scene's chunks were rewritten: rowids %s -> %s", keep, got)
	}
	var text string
	if err := st.DB().QueryRowContext(ctx, `SELECT text FROM manuscript_chunks WHERE node_id = ?`, second).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "바뀐 장면\n\n새 문단" {
		t.Fatalf("changed scene's chunk = %q", text)
	}
	if got := strings.Join(chunkRanges(t, ctx, idx, second), ","); got != "1-2" {
		t.Fatalf("changed scene's chunks = %s, want 1-2", got)
	}
}
