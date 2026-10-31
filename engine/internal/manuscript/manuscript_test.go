package manuscript

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func TestIndexerUpsertQueryDelete(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, indexer)

	sceneID := *p.LastOpenedNodeID
	doc := tiptapDoc("수아의 눈동자는 진홍빛이었다. 민호는 그 사실을 오래 기억했다.")
	if err := indexer.Upsert(ctx, p.ID, sceneID, doc); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	hits, err := searcher.Query(ctx, p.ID, "진홍빛", 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if hits[0].NodeID != sceneID {
		t.Fatalf("hit node = %q, want %q", hits[0].NodeID, sceneID)
	}
	if !strings.Contains(hits[0].Snippet, "진홍빛") {
		t.Fatalf("snippet missing query: %+v", hits[0])
	}
	if !strings.Contains(hits[0].Breadcrumb, "씬 1") {
		t.Fatalf("breadcrumb missing scene label: %+v", hits[0])
	}

	if err := indexer.Delete(ctx, sceneID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	hits, err = searcher.Query(ctx, p.ID, "진홍빛", 5)
	if err != nil {
		t.Fatalf("Query after delete: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits after delete = %d, want 0", len(hits))
	}
}

func TestSearcherLazyRebuildAndShortTermFallback(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, indexer)

	sceneID := *p.LastOpenedNodeID
	if err := nr.UpdateContent(ctx, sceneID, tiptapDoc("수아는 은색 열쇠를 주머니에 넣었다."), 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	hits, err := searcher.Query(ctx, p.ID, "수아", 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 1 || hits[0].NodeID != sceneID {
		t.Fatalf("short-term fallback hits = %+v, want scene %s", hits, sceneID)
	}
}

func TestIndexerRebuildIgnoresContainers(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, indexer)

	chapter, err := nr.CreateSibling(ctx, *p.LastOpenedNodeID, node.KindContainer, "1장", "색의 규칙", 2000)
	if err != nil {
		t.Fatalf("CreateSibling container: %v", err)
	}
	scene, err := nr.CreateChild(ctx, chapter.ID, node.KindLeaf, "씬 2", "붉은 방", 3000)
	if err != nil {
		t.Fatalf("CreateChild leaf: %v", err)
	}
	if err := nr.UpdateContent(ctx, scene.ID, tiptapDoc("붉은 방에는 오래된 시계가 있었다."), 4000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if err := indexer.Rebuild(ctx, p.ID); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	hits, err := searcher.Query(ctx, p.ID, "붉은 방", 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 1 || hits[0].NodeID != scene.ID {
		t.Fatalf("hits = %+v, want only leaf %s", hits, scene.ID)
	}
}

func TestIndexerRebuildAllRestoresLeavesAndRemovesOrphans(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	sceneID := *p.LastOpenedNodeID
	if err := nr.UpdateContent(ctx, sceneID, tiptapDoc("복구할 은빛 원고"), 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `INSERT INTO manuscript_fts (plain, node_id, project_id) VALUES ('orphan', 'missing', 'missing')`); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	if err := indexer.RebuildAll(ctx); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}
	var restored, orphan int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM manuscript_fts WHERE node_id = ? AND plain LIKE '%은빛%'`, sceneID).Scan(&restored); err != nil {
		t.Fatalf("count restored: %v", err)
	}
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM manuscript_fts WHERE node_id = 'missing'`).Scan(&orphan); err != nil {
		t.Fatalf("count orphan: %v", err)
	}
	if restored != 1 || orphan != 0 {
		t.Fatalf("restored=%d orphan=%d, want 1/0", restored, orphan)
	}
}

func TestBuildTrigramMatch(t *testing.T) {
	got, ok := buildTrigramMatch(searchTerms("진홍빛 눈동자"))
	if !ok {
		t.Fatal("buildTrigramMatch returned false for long Korean terms")
	}
	if got != `"진홍빛" AND "눈동자"` {
		t.Fatalf("query = %q", got)
	}

	got, ok = buildTrigramMatch(searchTerms(`진홍빛 OR "눈동자"`))
	if !ok {
		t.Fatal("buildTrigramMatch returned false for quoted/operator input")
	}
	if strings.Contains(got, " OR ") || strings.Contains(got, `"눈동자""`) {
		t.Fatalf("query did not neutralize operators/quotes: %q", got)
	}

	if _, ok := buildTrigramMatch(searchTerms("수아 민")); ok {
		t.Fatal("short-only terms should use LIKE fallback")
	}
	if _, ok := buildTrigramMatch(searchTerms("진홍빛 민호")); ok {
		t.Fatal("a short term must not be dropped from a mixed query")
	}
}

func TestSearchTermsKeepsPunctuationOnlyQuery(t *testing.T) {
	if got := searchTerms(`"()"`); len(got) != 1 || got[0] != `"()"` {
		t.Fatalf("terms = %q, want the raw query", got)
	}
}

func TestSearcherShortTermRanksByOccurrences(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, indexer)

	once := *p.LastOpenedNodeID
	often := sceneAfter(t, nr, once, "씬 2", "하린이 웃었다. 하린은 문을 열었고 하린의 손이 떨렸다.")
	if err := nr.UpdateContent(ctx, once, tiptapDoc("하린이 복도를 지나갔다."), 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if err := indexer.Rebuild(ctx, p.ID); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	hits, err := searcher.Query(ctx, p.ID, "하린", 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 2 || hits[0].NodeID != often || hits[1].NodeID != once {
		t.Fatalf("hits = %+v, want %s then %s", hits, often, once)
	}
}

func TestSearcherMixedQueryRequiresEveryWord(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, indexer)

	apart := *p.LastOpenedNodeID
	if err := nr.UpdateContent(ctx, apart, tiptapDoc("복수는 잊혔고 하린은 떠났다. 복수의 시간이었다."), 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	together := sceneAfter(t, nr, apart, "씬 2", "그날 밤 하린 복수를 다짐했다.")
	only := sceneAfter(t, nr, together, "씬 3", "하린이 차를 마셨다.")
	if err := indexer.Rebuild(ctx, p.ID); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	hits, err := searcher.Query(ctx, p.ID, "하린 복수", 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 2 || hits[0].NodeID != together || hits[1].NodeID != apart {
		t.Fatalf("hits = %+v, want phrase %s then %s, not %s", hits, together, apart, only)
	}

	hits, err = searcher.Query(ctx, p.ID, "다짐했다 하린", 5)
	if err != nil {
		t.Fatalf("Query mixed: %v", err)
	}
	if len(hits) != 1 || hits[0].NodeID != together {
		t.Fatalf("mixed hits = %+v, want only %s", hits, together)
	}
}

func TestSearcherShortTermSnippetIsWindowed(t *testing.T) {
	st, p, nr := manuscriptFixture(t)
	ctx := context.Background()
	indexer := NewIndexer(st.DB())
	searcher := NewSearcher(st.DB(), nr, indexer)

	padding := strings.Repeat("긴 문장이 이어진다. ", 200)
	sceneID := *p.LastOpenedNodeID
	if err := nr.UpdateContent(ctx, sceneID, tiptapDoc(padding+"수아가 나타났다. "+padding), 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if err := indexer.Rebuild(ctx, p.ID); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	hits, err := searcher.Query(ctx, p.ID, "수아", 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %+v, want 1", hits)
	}
	snippet := hits[0].Snippet
	if !strings.Contains(snippet, "수아") || !strings.HasPrefix(snippet, "...") || !strings.HasSuffix(snippet, "...") {
		t.Fatalf("snippet = %q", snippet)
	}
	if got, limit := utf8.RuneCountInString(snippet), 2*likeSnippetRadius+len([]rune("수아"))+6; got > limit {
		t.Fatalf("snippet is %d runes, want at most %d", got, limit)
	}
	if !utf8.ValidString(snippet) {
		t.Fatalf("snippet cut through a rune: %q", snippet)
	}
}

func TestLikeSnippetKeepsShortTextWhole(t *testing.T) {
	if got := likeSnippet("Sua met 수아", []string{"SUA"}); got != "Sua met 수아" {
		t.Fatalf("snippet = %q", got)
	}
}

func sceneAfter(t *testing.T, nr *node.Repo, afterID, label, text string) string {
	t.Helper()
	ctx := context.Background()
	scene, err := nr.CreateSibling(ctx, afterID, node.KindLeaf, label, "", 3000)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	if err := nr.UpdateContent(ctx, scene.ID, tiptapDoc(text), 4000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	return scene.ID
}

func manuscriptFixture(t *testing.T) (*store.Store, project.Project, *node.Repo) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pr := project.NewRepo(st)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"판타지"}, LengthTarget: "series", DefaultPOV: "third_limited",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return st, p, node.NewRepo(st)
}

func tiptapDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}
