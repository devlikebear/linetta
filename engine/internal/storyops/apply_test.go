package storyops

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func newTestService(t *testing.T) (*Service, *node.Repo, *snapshot.Repo, string, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	svc := New(projects, nodes, thread.NewRepo(st), beat.NewRepo(st),
		entity.NewRepo(st), relationship.NewRepo(st)).
		WithFacts(fact.NewRepo(st)).
		WithSnapshots(snaps)

	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title: "적용기 테스트", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return svc, nodes, snaps, p.ID, *p.LastOpenedNodeID
}

func now() func() int64 {
	var t int64 = 10_000
	return func() int64 { t++; return t }
}

// A structural batch applies atomically and leaves the writer one undo away
// from the pre-change outline.
func TestApplyOpsStructuralBatchAndUndo(t *testing.T) {
	svc, nodes, _, projectID, _ := newTestService(t)
	ctx := context.Background()
	clock := now()

	before, err := nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}

	res := svc.ApplyOps(ctx, projectID, "", Proposal{
		Summary: "1부 뼈대",
		Ops: []Op{
			{Type: "create_outline_node", Ref: "p1", Kind: "container", Label: "1부"},
			{Type: "create_outline_node", Ref: "s1", Kind: "leaf", ParentNodeRef: "p1", Label: "씬 1"},
		},
	}, clock)
	if res.IsError() {
		t.Fatalf("failures = %+v", res.Failures)
	}
	if res.Applied != 2 || res.UndoBatchID == "" {
		t.Fatalf("applied = %d, undo = %q; want 2 applied with an undo id", res.Applied, res.UndoBatchID)
	}
	after, _ := nodes.ListByProject(ctx, projectID)
	if len(after) != len(before)+2 {
		t.Fatalf("node count = %d, want %d", len(after), len(before)+2)
	}

	if err := svc.UndoApply(ctx, res.UndoBatchID, clock); err != nil {
		t.Fatalf("UndoApply: %v", err)
	}
	restored, _ := nodes.ListByProject(ctx, projectID)
	if len(restored) != len(before) {
		t.Fatalf("after undo node count = %d, want %d", len(restored), len(before))
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, clock); err != ErrUndoBatchNotFound {
		t.Fatalf("second undo = %v, want ErrUndoBatchNotFound", err)
	}
}

// A structural batch with a failing op is rolled back wholesale: half a
// restructured outline is worse than none.
func TestApplyOpsRollsBackFailedStructuralBatch(t *testing.T) {
	svc, nodes, _, projectID, _ := newTestService(t)
	ctx := context.Background()

	before, _ := nodes.ListByProject(ctx, projectID)
	res := svc.ApplyOps(ctx, projectID, "", Proposal{
		Ops: []Op{
			{Type: "create_outline_node", Kind: "container", Label: "1부"},
			{Type: "delete_outline_node", NodeID: "no-such-node"},
		},
	}, now())
	if !res.IsError() || !res.RolledBack {
		t.Fatalf("result = %+v, want failure with rollback", res)
	}
	if res.Applied != 0 || res.UndoBatchID != "" {
		t.Fatalf("rolled-back result must report nothing applied: %+v", res)
	}
	after, _ := nodes.ListByProject(ctx, projectID)
	if len(after) != len(before) {
		t.Fatalf("outline changed despite rollback: %d -> %d nodes", len(before), len(after))
	}
}

// set_scene_text records a companion-before snapshot when snapshots are wired,
// so every applied body change stays one restore away.
func TestApplyOpsSetSceneTextSnapshots(t *testing.T) {
	svc, nodes, snaps, projectID, sceneID := newTestService(t)
	ctx := context.Background()
	clock := now()

	seed, err := PlainTextToTiptapDoc("원래 본문")
	if err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	if err := nodes.UpdateContent(ctx, sceneID, seed, clock()); err != nil {
		t.Fatalf("seed content: %v", err)
	}

	res := svc.ApplyOps(ctx, projectID, sceneID, Proposal{
		Ops: []Op{{Type: "set_scene_text", Text: "고쳐 쓴 본문"}},
	}, clock)
	if res.IsError() {
		t.Fatalf("failures = %+v", res.Failures)
	}
	if len(res.ChangedNodes) != 1 || res.ChangedNodes[0].NodeID != sceneID {
		t.Fatalf("changed nodes = %+v", res.ChangedNodes)
	}

	got, _ := nodes.Get(ctx, sceneID)
	if !strings.Contains(*got.ContentDoc, "고쳐 쓴 본문") {
		t.Fatalf("scene body not replaced: %s", *got.ContentDoc)
	}
	list, err := snaps.ListForNode(ctx, sceneID)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	found := false
	for _, sn := range list {
		if sn.Reason == snapshot.ReasonCompanionBefore {
			found = true
		}
	}
	if !found {
		t.Fatalf("no companion-before snapshot recorded; got %+v", list)
	}
}

// Without a memory recorder the remember op must fail with a clear message,
// not a nil-pointer panic — the MCP applier is built without companion memory.
func TestApplyOpsRememberWithoutMemory(t *testing.T) {
	svc, _, _, projectID, _ := newTestService(t)
	res := svc.ApplyOps(context.Background(), projectID, "", Proposal{
		Ops: []Op{{Type: "remember", Text: "작가는 건조한 문체를 선호한다"}},
	}, now())
	if !res.IsError() {
		t.Fatal("remember without memory should fail")
	}
	if !strings.Contains(res.Failures[0].Error, "memory is not available") {
		t.Fatalf("failure = %+v, want a clear memory-unavailable message", res.Failures[0])
	}
}

// story-assist Phase 3 (3.A.2): create_entity sets aliases, update_entity
// only adds to them, and a seeded Refs lets a later call resolve a ref an
// earlier batch declared.

func TestApplyOpsCreateEntitySetsAliases(t *testing.T) {
	svc, _, _, projectID, _ := newTestService(t)
	res := svc.ApplyOps(context.Background(), projectID, "", Proposal{
		Ops: []Op{{Type: "create_entity", Ref: "e1", Name: "하린", Aliases: []string{"Ha-rin", " 하린 "}}},
	}, now())
	if res.IsError() {
		t.Fatalf("failures = %+v", res.Failures)
	}
	entID := res.Created["entity:e1"]
	if entID == "" {
		t.Fatal("no created entity id recorded")
	}
	ent, err := svc.entities.Get(context.Background(), entID)
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	if len(ent.Aliases) != 2 || ent.Aliases[0] != "Ha-rin" || ent.Aliases[1] != "하린" {
		t.Fatalf("aliases = %+v, want [Ha-rin 하린] (trimmed, in order)", ent.Aliases)
	}
}

func TestApplyOpsUpdateEntityAddsAliasesWithoutDroppingExisting(t *testing.T) {
	svc, _, _, projectID, _ := newTestService(t)
	ctx := context.Background()
	create := svc.ApplyOps(ctx, projectID, "", Proposal{
		Ops: []Op{{Type: "create_entity", Ref: "e1", Name: "하린", Aliases: []string{"Ha-rin"}}},
	}, now())
	if create.IsError() {
		t.Fatalf("create failures = %+v", create.Failures)
	}
	entID := create.Created["entity:e1"]

	update := svc.ApplyOps(ctx, projectID, "", Proposal{
		Ops: []Op{{Type: "update_entity", EntityID: entID, Aliases: []string{"HA-RIN", "검술 사범"}}},
	}, now())
	if update.IsError() {
		t.Fatalf("update failures = %+v", update.Failures)
	}
	ent, err := svc.entities.Get(ctx, entID)
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	// "HA-RIN" case-insensitively duplicates "Ha-rin" and must not be added
	// twice; "검술 사범" is new and must be appended; "Ha-rin" must survive.
	if len(ent.Aliases) != 2 {
		t.Fatalf("aliases = %+v, want exactly 2 (Ha-rin kept, HA-RIN deduped, 검술 사범 added)", ent.Aliases)
	}
	if ent.Aliases[0] != "Ha-rin" {
		t.Errorf("aliases[0] = %q, want original alias kept first", ent.Aliases[0])
	}
	foundNew := false
	for _, a := range ent.Aliases {
		if a == "검술 사범" {
			foundNew = true
		}
		if strings.EqualFold(a, "HA-RIN") && a != "Ha-rin" {
			t.Errorf("duplicate case-insensitive alias %q added alongside %q", a, "Ha-rin")
		}
	}
	if !foundNew {
		t.Fatalf("aliases = %+v, want 검술 사범 added", ent.Aliases)
	}
}

func TestApplyOpsWithRefsSeedResolvesRelationship(t *testing.T) {
	svc, _, _, projectID, _ := newTestService(t)
	ctx := context.Background()

	// An earlier call created this entity and is the ref's owner; a later
	// call only has the resulting real id, seeded in as if it were the ref.
	ent, err := svc.entities.Create(ctx, 1_000, entity.NewInput{ProjectID: projectID, Name: "하린"})
	if err != nil {
		t.Fatalf("seed entity: %v", err)
	}

	res, refs := svc.ApplyOpsWithRefs(ctx, projectID, "", Proposal{
		Ops: []Op{
			{Type: "create_entity", Ref: "e2", Name: "유나"},
			{Type: "create_relationship", FromRef: "e1", ToRef: "e2", Label: "스승"},
		},
	}, Refs{Entities: map[string]string{"e1": ent.ID}}, now())
	if res.IsError() {
		t.Fatalf("failures = %+v", res.Failures)
	}
	if res.Applied != 2 {
		t.Fatalf("applied = %d, want 2", res.Applied)
	}
	if refs.Entities["e1"] != ent.ID {
		t.Errorf("seeded ref e1 = %q, want preserved as %q", refs.Entities["e1"], ent.ID)
	}
	if refs.Entities["e2"] == "" {
		t.Error("new ref e2 was not recorded in the returned Refs")
	}
}

func mustApply(t *testing.T, svc *Service, ctx context.Context, projectID, nodeID string, p Proposal, clock func() int64) ApplyOpsResult {
	t.Helper()
	res := svc.ApplyOps(ctx, projectID, nodeID, p, clock)
	if res.Applied != len(p.Ops) {
		t.Fatalf("applied %d of %d: %+v", res.Applied, len(p.Ops), res)
	}
	return res
}

func outlineFixture(t *testing.T) (*Service, *node.Repo, string, string, string) {
	t.Helper()
	svc, nodes, _, projectID, first := newTestService(t)
	res := mustApply(t, svc, context.Background(), projectID, first, Proposal{Ops: []Op{
		{Type: "create_outline_node", Ref: "p1", Kind: "container", Label: "1부"},
		{Type: "create_outline_node", Ref: "p2", Kind: "container", Label: "2부"},
	}}, now())
	return svc, nodes, projectID, res.Created["node:p1"], res.Created["node:p2"]
}

func TestCreateSceneLandsInTheNamedPart(t *testing.T) {
	svc, nodes, projectID, _, part2 := outlineFixture(t)
	ctx := context.Background()
	res := mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{
		{Type: "create_scene", Ref: "s", Label: "9화", ParentNodeID: part2},
		{Type: "create_scene", Ref: "s2", Label: "10화", ParentNodeID: part2},
	}}, now())
	for _, ref := range []string{"node:s", "node:s2"} {
		n, err := nodes.Get(ctx, res.Created[ref])
		if err != nil {
			t.Fatal(err)
		}
		if n.ParentID == nil || *n.ParentID != part2 {
			t.Fatalf("%s parent = %v, want %s", ref, n.ParentID, part2)
		}
	}
}

func TestCreateSceneTakesAParentRefFromTheSameBatch(t *testing.T) {
	svc, nodes, _, projectID, first := newTestService(t)
	ctx := context.Background()
	res := mustApply(t, svc, ctx, projectID, first, Proposal{Ops: []Op{
		{Type: "create_outline_node", Ref: "p", Kind: "container", Label: "3부"},
		{Type: "create_scene", Ref: "s", Label: "1화", ParentNodeRef: "p"},
	}}, now())
	n, _ := nodes.Get(ctx, res.Created["node:s"])
	if n.ParentID == nil || *n.ParentID != res.Created["node:p"] {
		t.Fatalf("parent = %v", n.ParentID)
	}
}

func TestCreateSceneRefusesAParentThatIsAScene(t *testing.T) {
	svc, _, _, projectID, first := newTestService(t)
	res := svc.ApplyOps(context.Background(), projectID, first, Proposal{Ops: []Op{
		{Type: "create_scene", Label: "x", ParentNodeID: first},
	}}, now())
	if res.Applied != 0 || len(res.Failures) != 1 || !strings.Contains(res.Failures[0].Error, "container") {
		t.Fatalf("res = %+v", res)
	}
}

func TestCreateKeepsThePlanGivenAsSummary(t *testing.T) {
	svc, nodes, projectID, part1, _ := outlineFixture(t)
	ctx := context.Background()
	res := mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{
		{Type: "create_scene", Ref: "s", Label: "1화", ParentNodeID: part1, Summary: "[계획] 첫 화"},
		{Type: "create_outline_node", Ref: "o", Kind: "leaf", Label: "2화", ParentNodeID: part1, Summary: "[계획] 둘째 화"},
	}}, now())
	for ref, want := range map[string]string{"node:s": "[계획] 첫 화", "node:o": "[계획] 둘째 화"} {
		n, _ := nodes.Get(ctx, res.Created[ref])
		if n.Summary != want || !n.SummaryWritten {
			t.Fatalf("%s summary = %q written=%v", ref, n.Summary, n.SummaryWritten)
		}
	}
}

func TestRenameLabelCarriesATitleThatRepeatedIt(t *testing.T) {
	svc, nodes, projectID, part1, _ := outlineFixture(t)
	ctx := context.Background()
	if err := nodes.Rename(ctx, part1, "1부", "1부", 20_000); err != nil {
		t.Fatal(err)
	}
	mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{{Type: "rename_outline_node", NodeID: part1, Label: "1부. 시작"}}}, now())
	n, _ := nodes.Get(ctx, part1)
	if n.Title != "1부. 시작" {
		t.Fatalf("title = %q", n.Title)
	}
}

func TestRenameLabelLeavesAnOwnTitleAlone(t *testing.T) {
	svc, nodes, projectID, part1, _ := outlineFixture(t)
	ctx := context.Background()
	if err := nodes.Rename(ctx, part1, "1부", "가짜 명패", 20_000); err != nil {
		t.Fatal(err)
	}
	mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{{Type: "rename_outline_node", NodeID: part1, Label: "제1부"}}}, now())
	n, _ := nodes.Get(ctx, part1)
	if n.Title != "가짜 명패" {
		t.Fatalf("title = %q", n.Title)
	}
}

func TestMoveIntoAnotherPartAndUndoIt(t *testing.T) {
	svc, nodes, projectID, part1, part2 := outlineFixture(t)
	ctx := context.Background()
	res := mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{
		{Type: "create_scene", Ref: "s", Label: "1화", ParentNodeID: part1},
	}}, now())
	scene := res.Created["node:s"]
	moved := mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{
		{Type: "move_outline_node", NodeID: scene, ParentNodeID: part2},
	}}, now())
	n, _ := nodes.Get(ctx, scene)
	if n.ParentID == nil || *n.ParentID != part2 {
		t.Fatalf("parent after move = %v", n.ParentID)
	}
	if err := svc.UndoApply(ctx, moved.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	n, _ = nodes.Get(ctx, scene)
	if n.ParentID == nil || *n.ParentID != part1 {
		t.Fatalf("parent after undo = %v", n.ParentID)
	}
}

func TestMoveNeedsADirectionOrAParent(t *testing.T) {
	for _, op := range []Op{
		{Type: "move_outline_node", NodeID: "a"},
		{Type: "move_outline_node", NodeID: "a", ParentNodeID: "b", Direction: "up"},
	} {
		if err := ValidateProposal(Proposal{Summary: "s", Ops: []Op{op}}); err == nil {
			t.Fatalf("op %+v was accepted", op)
		}
	}
}

func TestCreateThreadTakesTitleAsItsName(t *testing.T) {
	p := Proposal{Summary: "s", Ops: []Op{{Type: "create_thread", Title: "복선 줄기"}}}
	if err := ValidateProposal(p); err != nil {
		t.Fatalf("ValidateProposal: %v", err)
	}
	if p.Ops[0].Name != "복선 줄기" {
		t.Fatalf("name = %q", p.Ops[0].Name)
	}
}

type recordedCuts struct{ cut []node.Node }

func (r *recordedCuts) RecordCutPlans(_ context.Context, _ int64, cut []node.Node) ([]string, error) {
	r.cut = cut
	return []string{cut[0].Label}, nil
}

func TestDeletingAPlannedEpisodeReportsItsPlan(t *testing.T) {
	svc, nodes, projectID, part1, _ := outlineFixture(t)
	ctx := context.Background()
	rec := &recordedCuts{}
	svc.WithCutPlans(rec)
	res := mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{
		{Type: "create_scene", Ref: "s", Label: "20화", ParentNodeID: part1, Plan: "[계획] 스무 번째"},
	}}, now())
	scene := res.Created["node:s"]
	n, _ := nodes.Get(ctx, scene)
	if n.Plan != "[계획] 스무 번째" {
		t.Fatalf("plan = %q", n.Plan)
	}
	del := mustApply(t, svc, ctx, projectID, "", Proposal{Ops: []Op{{Type: "delete_outline_node", NodeID: scene}}}, now())
	if len(del.CutPlans) != 1 || del.CutPlans[0] != "20화" || len(rec.cut) != 1 {
		t.Fatalf("cut plans = %v, recorded %d", del.CutPlans, len(rec.cut))
	}
	if err := svc.UndoApply(ctx, del.UndoBatchID, now()); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if n, _ := nodes.Get(ctx, scene); n.Plan != "[계획] 스무 번째" {
		t.Fatalf("undo lost the plan: %q", n.Plan)
	}
}
