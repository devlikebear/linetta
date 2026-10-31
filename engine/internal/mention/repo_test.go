package mention

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	store *store.Store
	mr    *Repo
	er    *entity.Repo
	nr    *node.Repo
	pID   string
	nID   string
}

func newFixture(t *testing.T) fixture {
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
	return fixture{store: s, mr: NewRepo(s), er: entity.NewRepo(s), nr: node.NewRepo(s), pID: p.ID, nID: *p.LastOpenedNodeID}
}

func TestResyncForNode_insertsValidMentions_dropsUnknown(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진"})

	found := []Found{
		{EntityID: e.ID, Position: 5, Surface: "해진"},
		{EntityID: "missing-uuid", Position: 10, Surface: "윤서"}, // dropped (FK invalid)
	}
	if err := f.mr.ResyncForNode(ctx, f.nID, found); err != nil {
		t.Fatalf("ResyncForNode: %v", err)
	}
	got, err := f.mr.ListForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	if len(got) != 1 || got[0].EntityID != e.ID {
		t.Errorf("after resync: %+v", got)
	}
}

func TestResyncForNode_replacesPreviousSet(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "A"})
	b, _ := f.er.Create(ctx, 110, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "B"})

	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: a.ID, Position: 1, Surface: "A"}})
	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: b.ID, Position: 1, Surface: "B"}})

	got, _ := f.mr.ListForNode(ctx, f.nID)
	if len(got) != 1 || got[0].EntityID != b.ID {
		t.Errorf("after re-resync: %+v", got)
	}
}

func TestRebuildAll_restoresMentionsFromPersistedDocuments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, err := f.er.Create(ctx, 1, entity.NewInput{ProjectID: f.pID, Name: "해진", Kind: entity.KindCharacter})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"` + e.ID + `","label":"해진"}}]}]}`
	if err := f.nr.UpdateContent(ctx, f.nID, doc, 2); err != nil {
		t.Fatalf("update content: %v", err)
	}
	if got, err := f.mr.ListForNode(ctx, f.nID); err != nil || len(got) != 0 {
		t.Fatalf("mentions should start empty: got=%v err=%v", got, err)
	}

	if err := f.mr.RebuildAll(ctx); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}
	got, err := f.mr.ListForNode(ctx, f.nID)
	if err != nil || len(got) != 1 || got[0].EntityID != e.ID {
		t.Fatalf("rebuilt mentions = %+v, err=%v", got, err)
	}
}

func TestListEntitiesForNode_hydratesEntities(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진", Role: "POV"})
	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: e.ID, Position: 1, Surface: "해진"}})

	got, err := f.mr.ListEntitiesForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListEntitiesForNode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Name != "해진" || got[0].Role != "POV" {
		t.Errorf("entity hydration missed: %+v", got[0])
	}
}

func TestMentionedNodeIDs(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	n1 := f.nID
	nr := node.NewRepo(f.store)
	n2node, err := nr.CreateSibling(ctx, n1, "leaf", "씬 2", "", 1100)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	n2 := n2node.ID

	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진"})

	// n1 mentions e once.
	if err := f.mr.ResyncForNode(ctx, n1, []Found{{EntityID: e.ID, Position: 1, Surface: "해진"}}); err != nil {
		t.Fatalf("ResyncForNode n1: %v", err)
	}
	// n2 mentions e twice in one call -> 2 mention rows for n2.
	if err := f.mr.ResyncForNode(ctx, n2, []Found{
		{EntityID: e.ID, Position: 1, Surface: "해진"},
		{EntityID: e.ID, Position: 9, Surface: "해진"},
	}); err != nil {
		t.Fatalf("ResyncForNode n2: %v", err)
	}

	ids, projectID, err := f.mr.MentionedNodeIDs(ctx, e.ID)
	if err != nil {
		t.Fatalf("MentionedNodeIDs: %v", err)
	}
	if projectID != f.pID {
		t.Fatalf("projectID=%q want %q", projectID, f.pID)
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	if len(ids) != 2 || !set[n1] || !set[n2] {
		t.Fatalf("ids=%v want distinct {n1=%q, n2=%q}", ids, n1, n2)
	}
}

func TestMentionedNodeIDs_noMentions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진"})

	ids, projectID, err := f.mr.MentionedNodeIDs(ctx, e.ID)
	if err != nil {
		t.Fatalf("MentionedNodeIDs: %v", err)
	}
	if projectID != f.pID {
		t.Fatalf("projectID=%q want %q", projectID, f.pID)
	}
	if len(ids) != 0 {
		t.Fatalf("ids=%v want empty", ids)
	}
}

// summarizedScene adds a leaf after afterID that mentions the entities (an
// entity listed twice is mentioned twice) and has a summary, as CoMentionLeaves
// requires.
func summarizedScene(t *testing.T, f fixture, afterID string, at int64, entityIDs ...string) string {
	t.Helper()
	ctx := context.Background()
	n, err := f.nr.CreateSibling(ctx, afterID, "leaf", "씬", "", at)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	if err := f.nr.UpdateContent(ctx, n.ID, `{"type":"doc","content":[{"type":"paragraph"}]}`, at); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	got, err := f.nr.Get(ctx, n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := f.nr.SetSummary(ctx, n.ID, "요약", got.ContentVersion); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}
	mentionAll(t, f, n.ID, entityIDs...)
	return n.ID
}

func mentionAll(t *testing.T, f fixture, nodeID string, entityIDs ...string) {
	t.Helper()
	found := make([]Found, 0, len(entityIDs))
	for i, id := range entityIDs {
		found = append(found, Found{EntityID: id, Position: i, Surface: "x"})
	}
	if err := f.mr.ResyncForNode(context.Background(), nodeID, found); err != nil {
		t.Fatalf("ResyncForNode: %v", err)
	}
}

func TestCoMentionLeaves_rareEntityOutranksLeads(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	lead1, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진"})
	lead2, _ := f.er.Create(ctx, 101, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "민호"})
	prop, _ := f.er.Create(ctx, 102, entity.NewInput{ProjectID: f.pID, Kind: "item", Name: "은색 열쇠"})

	mentionAll(t, f, f.nID, lead1.ID, lead2.ID, prop.ID)
	rare := summarizedScene(t, f, f.nID, 200, lead1.ID, prop.ID)
	// Both leads and edited later: shared count and recency both favor this one.
	leads := summarizedScene(t, f, rare, 300, lead1.ID, lead2.ID)
	summarizedScene(t, f, leads, 400, lead1.ID, lead2.ID)

	got, err := f.mr.CoMentionLeaves(ctx, f.nID, 10)
	if err != nil {
		t.Fatalf("CoMentionLeaves: %v", err)
	}
	if len(got) != 3 || got[0].NodeID != rare {
		t.Fatalf("results = %+v, want %s first", got, rare)
	}
}

func TestCoMentionLeaves_repeatedMentionsCountOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e1, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진"})
	e2, _ := f.er.Create(ctx, 101, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "민호"})

	mentionAll(t, f, f.nID, e1.ID, e1.ID, e2.ID)
	once := summarizedScene(t, f, f.nID, 200, e1.ID, e2.ID)
	repeated := summarizedScene(t, f, once, 100, e1.ID, e1.ID, e1.ID, e1.ID, e2.ID)

	got, err := f.mr.CoMentionLeaves(ctx, f.nID, 10)
	if err != nil {
		t.Fatalf("CoMentionLeaves: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("results = %+v, want 2", got)
	}
	want := 2.0 / 3.0 // each entity appears in 3 scenes
	for _, r := range got {
		if r.K != 2 || math.Abs(r.Rarity-want) > 1e-9 {
			t.Errorf("%+v: want K=2 Rarity=%v", r, want)
		}
	}
	if got[0].NodeID != once || got[1].NodeID != repeated {
		t.Errorf("order = %+v, want the tie broken by recency", got)
	}
}
