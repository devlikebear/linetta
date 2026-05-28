package node

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newStoreAndProject(t *testing.T) (*store.Store, project.Project) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return s, p
}

func TestRepo_Get_firstLeaf(t *testing.T) {
	s, p := newStoreAndProject(t)
	if p.LastOpenedNodeID == nil {
		t.Fatal("project has no first leaf")
	}
	r := NewRepo(s)
	n, err := r.Get(context.Background(), *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Kind != "leaf" {
		t.Errorf("kind = %q, want leaf", n.Kind)
	}
	if n.Label != "씬 1" {
		t.Errorf("label = %q, want 씬 1", n.Label)
	}
	if n.ContentDoc == nil {
		t.Fatal("first leaf has no content_doc")
	}
}

func TestRepo_UpdateContent_updatesWordCount_andProjectCount(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	pr := project.NewRepo(s)
	ctx := context.Background()

	// Insert content with 5 visible characters.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"안녕 세계"}]}]}`
	if err := r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 9999); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	got, err := r.Get(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WordCount != 5 {
		t.Errorf("node.word_count = %d, want 5", got.WordCount)
	}
	if got.UpdatedAt != 9999 {
		t.Errorf("node.updated_at = %d, want 9999", got.UpdatedAt)
	}

	pp, err := pr.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if pp.WordCount != 5 {
		t.Errorf("project.word_count = %d, want 5", pp.WordCount)
	}
	if pp.UpdatedAt != 9999 {
		t.Errorf("project.updated_at = %d, want 9999", pp.UpdatedAt)
	}
}

func TestRepo_SetLastOpened(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	pr := project.NewRepo(s)
	ctx := context.Background()

	original := *p.LastOpenedNodeID
	if err := r.SetLastOpened(ctx, p.ID, original, 1234); err != nil {
		t.Fatalf("SetLastOpened: %v", err)
	}

	pp, err := pr.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if pp.LastOpenedNodeID == nil || *pp.LastOpenedNodeID != original {
		t.Errorf("last_opened_node_id = %v, want %q", pp.LastOpenedNodeID, original)
	}
	if pp.UpdatedAt != 1234 {
		t.Errorf("project.updated_at = %d, want 1234", pp.UpdatedAt)
	}
}

func TestRepo_UpdateContent_rejectsMissingNode(t *testing.T) {
	s, _ := newStoreAndProject(t)
	r := NewRepo(s)
	err := r.UpdateContent(context.Background(), "no-such-id", `{"type":"doc"}`, 1)
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepo_ListByProject_returnsTreeOrdered(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	// Project starts with one leaf ("씬 1"). Add a container sibling and a leaf inside it.
	chapter, err := r.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1장", "", 2000)
	if err != nil {
		t.Fatalf("CreateSibling chapter: %v", err)
	}
	if _, err := r.CreateChild(ctx, chapter.ID, "leaf", "씬 A", "", 3000); err != nil {
		t.Fatalf("CreateChild leaf: %v", err)
	}

	list, err := r.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if got, want := len(list), 3; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	// First two rows: project-root siblings ordered by ordinal.
	if list[0].Label != "씬 1" || list[1].Label != "1장" {
		t.Errorf("root order = %q, %q; want 씬 1, 1장", list[0].Label, list[1].Label)
	}
	// Third row: leaf inside chapter.
	if list[2].ParentID == nil || *list[2].ParentID != chapter.ID {
		t.Errorf("third row not under chapter")
	}
}

func TestRepo_CreateSibling_placesAfterReference(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	created, err := r.CreateSibling(ctx, *p.LastOpenedNodeID, "leaf", "씬 2", "", 2000)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	if created.Ordinal != 1 {
		t.Errorf("ordinal = %d, want 1 (after 씬 1)", created.Ordinal)
	}
	if created.ParentID != nil {
		t.Errorf("expected top-level node, got parent_id = %v", created.ParentID)
	}
}

func TestRepo_CreateChild_lastOrdinal(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	chapter, _ := r.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1장", "", 2000)
	first, err := r.CreateChild(ctx, chapter.ID, "leaf", "씬 A", "", 3000)
	if err != nil {
		t.Fatalf("CreateChild: %v", err)
	}
	if first.Ordinal != 0 {
		t.Errorf("first child ordinal = %d, want 0", first.Ordinal)
	}
	second, _ := r.CreateChild(ctx, chapter.ID, "leaf", "씬 B", "", 4000)
	if second.Ordinal != 1 {
		t.Errorf("second child ordinal = %d, want 1", second.Ordinal)
	}
}

func TestRepo_Rename_updatesLabelAndTitle(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	if err := r.Rename(ctx, *p.LastOpenedNodeID, "프롤로그", "별이 떨어지는 밤", 5000); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if got.Label != "프롤로그" {
		t.Errorf("label = %q", got.Label)
	}
	if got.Title != "별이 떨어지는 밤" {
		t.Errorf("title = %q", got.Title)
	}
	if got.UpdatedAt != 5000 {
		t.Errorf("updated_at = %d", got.UpdatedAt)
	}
}

func TestRepo_Delete_removesNode_andUpdatesProjectWordCount(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	// Write some content so word_count is non-zero, then delete.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"세 글자"}]}]}`
	if err := r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	if err := r.Delete(ctx, *p.LastOpenedNodeID, 3000); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, *p.LastOpenedNodeID); err != ErrNotFound {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}

	// Project word_count should reflow to 0.
	var pcount int
	if err := s.DB().QueryRowContext(ctx, `SELECT word_count FROM projects WHERE id = ?`, p.ID).Scan(&pcount); err != nil {
		t.Fatalf("project count: %v", err)
	}
	if pcount != 0 {
		t.Errorf("project.word_count = %d, want 0", pcount)
	}
}

func TestRepo_MoveUp_andMoveDown(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	first := *p.LastOpenedNodeID                                                          // ordinal 0
	second, _ := r.CreateSibling(ctx, first, "leaf", "씬 2", "", 2000)                    // ordinal 1
	third, _ := r.CreateSibling(ctx, second.ID, "leaf", "씬 3", "", 3000)                 // ordinal 2

	// Move third up → should swap with second.
	if err := r.MoveUp(ctx, third.ID, 4000); err != nil {
		t.Fatalf("MoveUp: %v", err)
	}
	list, _ := r.ListByProject(ctx, p.ID)
	if list[0].Label != "씬 1" || list[1].Label != "씬 3" || list[2].Label != "씬 2" {
		t.Errorf("after MoveUp(third) = %q,%q,%q; want 씬 1, 씬 3, 씬 2",
			list[0].Label, list[1].Label, list[2].Label)
	}

	// Move first down → swap with what's now in slot 1 (씬 3).
	if err := r.MoveDown(ctx, first, 5000); err != nil {
		t.Fatalf("MoveDown: %v", err)
	}
	list, _ = r.ListByProject(ctx, p.ID)
	if list[0].Label != "씬 3" || list[1].Label != "씬 1" {
		t.Errorf("after MoveDown(first) = %q,%q,...", list[0].Label, list[1].Label)
	}

	// MoveUp on the first-position node is a no-op (no error).
	if err := r.MoveUp(ctx, list[0].ID, 6000); err != nil {
		t.Errorf("MoveUp on first: %v", err)
	}
}

func TestRepo_UpdateContent_bumpsContentVersion(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	before, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if before.ContentVersion != 0 {
		t.Fatalf("seeded content_version = %d, want 0", before.ContentVersion)
	}

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"한"}]}]}`
	_ = r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 9999)
	_ = r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 10000)

	after, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if after.ContentVersion != 2 {
		t.Errorf("content_version = %d, want 2", after.ContentVersion)
	}
}

func TestRepo_UpdateContent_bumpsAncestorContentVersion(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	// 부 → 장 → 씬 tree built off the seeded first leaf.
	part, err := r.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", 1100)
	if err != nil {
		t.Fatalf("part: %v", err)
	}
	chapter, err := r.CreateChild(ctx, part.ID, "container", "1장", "", 1110)
	if err != nil {
		t.Fatalf("chapter: %v", err)
	}
	scene, err := r.CreateChild(ctx, chapter.ID, "leaf", "씬 1", "", 1120)
	if err != nil {
		t.Fatalf("scene: %v", err)
	}

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"한"}]}]}`
	if err := r.UpdateContent(ctx, scene.ID, doc, 1200); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	gotScene, _ := r.Get(ctx, scene.ID)
	gotChap, _ := r.Get(ctx, chapter.ID)
	gotPart, _ := r.Get(ctx, part.ID)
	if gotScene.ContentVersion != 1 {
		t.Errorf("scene.content_version = %d, want 1", gotScene.ContentVersion)
	}
	if gotChap.ContentVersion != 1 {
		t.Errorf("chapter.content_version = %d, want 1 (ancestor bumped)", gotChap.ContentVersion)
	}
	if gotPart.ContentVersion != 1 {
		t.Errorf("part.content_version = %d, want 1 (ancestor bumped)", gotPart.ContentVersion)
	}
	if gotChap.SummaryForVersion != 0 || gotPart.SummaryForVersion != 0 {
		t.Errorf("ancestor summary_for_version should still be 0 (stale): chap=%d part=%d",
			gotChap.SummaryForVersion, gotPart.SummaryForVersion)
	}

	// Second write bumps all three again.
	if err := r.UpdateContent(ctx, scene.ID, doc, 1300); err != nil {
		t.Fatalf("UpdateContent#2: %v", err)
	}
	gotChap2, _ := r.Get(ctx, chapter.ID)
	if gotChap2.ContentVersion != 2 {
		t.Errorf("after second write, chapter.content_version = %d, want 2", gotChap2.ContentVersion)
	}
}

func TestRepo_ListChildren_returnsChildrenInOrdinalOrder(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	chap, _ := r.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1장", "", 2000)
	a, _ := r.CreateChild(ctx, chap.ID, "leaf", "씬 A", "", 2100)
	b, _ := r.CreateChild(ctx, chap.ID, "leaf", "씬 B", "", 2200)

	children, err := r.ListChildren(ctx, chap.ID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("len = %d, want 2", len(children))
	}
	if children[0].ID != a.ID || children[1].ID != b.ID {
		t.Errorf("order = %q,%q; want %q,%q",
			children[0].Label, children[1].Label, a.Label, b.Label)
	}
}

func TestRepo_SetSummary_writesBothFields(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}`
	for i := 0; i < 3; i++ {
		_ = r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, int64(1000+i))
	}
	if err := r.SetSummary(ctx, *p.LastOpenedNodeID, "요약된 본문.", 3); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}
	got, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if got.Summary != "요약된 본문." || got.SummaryForVersion != 3 || got.ContentVersion != 3 {
		t.Errorf("got = %+v", got)
	}
}

func TestRepo_SetSummary_unknownID_returnsErrNotFound(t *testing.T) {
	s, _ := newStoreAndProject(t)
	r := NewRepo(s)
	if err := r.SetSummary(context.Background(), "no-such", "x", 1); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepo_UpdateContent_callsResyncer(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	called := 0
	var gotDoc, gotID string
	r.SetMentionResyncer(func(_ context.Context, nodeID, doc string) error {
		called++
		gotID = nodeID
		gotDoc = doc
		return nil
	})
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`
	if err := r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 9999); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if called != 1 {
		t.Errorf("resyncer called %d times, want 1", called)
	}
	if gotID != *p.LastOpenedNodeID || gotDoc != doc {
		t.Errorf("resyncer args wrong: id=%q docLen=%d", gotID, len(gotDoc))
	}
}
