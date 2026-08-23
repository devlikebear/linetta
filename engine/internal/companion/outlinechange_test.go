package companion

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func outlineOpsJSON(count int) string {
	ops := make([]string, 0, count+1)
	ops = append(ops, `{"op":"create_outline_node","ref":"p1","kind":"container","label":"1부","title":"항구의 복수극"}`)
	for i := 1; i <= count; i++ {
		ops = append(ops, `{"op":"create_outline_node","ref":"s`+strconv.Itoa(i)+`","kind":"leaf","parent_node_ref":"p1","label":"씬 `+strconv.Itoa(i)+`"}`)
	}
	return "[" + strings.Join(ops, ",") + "]"
}

func outlineApplyParams(t *testing.T, summary, opsJSON string) json.RawMessage {
	t.Helper()
	return json.RawMessage(`{"summary":` + strconv.Quote(summary) + `,"ops_json":` + strconv.Quote(opsJSON) + `}`)
}

func TestNeedsOutlineApprovalOnlyForStructuralBatches(t *testing.T) {
	small := Proposal{Ops: []Op{
		{Type: "create_scene", Label: "씬 2"},
		{Type: "add_beat", Label: "단서 발견"},
	}}
	if needsOutlineApproval(small) {
		t.Fatal("a two-op edit should apply without an approval step")
	}

	chatty := Proposal{Ops: []Op{
		{Type: "remember", Text: "작가는 냉소적인 문체를 좋아한다"},
		{Type: "create_thread", Name: "복수극"},
		{Type: "add_beat", Label: "결심"},
		{Type: "add_beat", Label: "추적"},
		{Type: "add_beat", Label: "대면"},
		{Type: "add_beat", Label: "결말"},
	}}
	if needsOutlineApproval(chatty) {
		t.Fatal("beats and memories do not reshape the outline")
	}

	big := Proposal{}
	for i := 0; i < largeOutlineChangeThreshold; i++ {
		big.Ops = append(big.Ops, Op{Type: "create_outline_node", Label: "화"})
	}
	if !needsOutlineApproval(big) {
		t.Fatalf("%d outline creations should need approval", largeOutlineChangeThreshold)
	}
}

func TestBuildOutlinePreviewCountsAndNests(t *testing.T) {
	svc, projectID, _ := newToolSvc(t)
	p := Proposal{Summary: "1부 구성", Ops: []Op{
		{Type: "create_outline_node", Ref: "p1", Kind: "container", Label: "1부"},
		{Type: "create_outline_node", Ref: "c1", Kind: "container", ParentNodeRef: "p1", Label: "1화"},
		{Type: "create_outline_node", Ref: "s1", Kind: "leaf", ParentNodeRef: "c1", Label: "씬 1", Title: "안개 낀 항구"},
		{Type: "add_beat", Label: "단서 발견"},
	}}

	preview := svc.buildOutlinePreview(context.Background(), projectID, p)

	if preview.Counts.Created != 3 || preview.Counts.Other != 1 {
		t.Fatalf("counts = %+v, want 3 created and 1 other", preview.Counts)
	}
	if preview.Counts.Structural() != 3 {
		t.Fatalf("structural count = %d, want 3", preview.Counts.Structural())
	}
	if len(preview.Tree) != 3 {
		t.Fatalf("tree should only list outline rows: %+v", preview.Tree)
	}
	if preview.Tree[0].Depth != 0 || preview.Tree[1].Depth != 1 || preview.Tree[2].Depth != 2 {
		t.Fatalf("tree depths = %d/%d/%d, want 0/1/2",
			preview.Tree[0].Depth, preview.Tree[1].Depth, preview.Tree[2].Depth)
	}
	if preview.Tree[2].Title != "안개 낀 항구" || preview.Tree[2].Action != "create" {
		t.Fatalf("leaf row = %+v", preview.Tree[2])
	}
	if len(preview.Ops) != 4 {
		t.Fatalf("preview should carry the ops to apply later: %+v", preview.Ops)
	}
}

// A big restructure is shown to the writer instead of landing on the project.
func TestLinettaApplyOpsToolPreviewsLargeOutlineChanges(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	notif := &fakeNotifier{}
	svc.notify = notif
	userText := "작품 전체 아웃라인 구성해줘"
	intent := classifyCompanionIntent(userText)
	tool := svc.buildApplyOpsTool(projectID, nodeID, HistoryScopeProject, "run-1", userText, intent, "", func() int64 { return 1 })

	before, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}

	result, err := tool.Execute(ctx, outlineApplyParams(t, "1부 아웃라인", outlineOpsJSON(8)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("a preview is not a failure: %s", result.Text())
	}
	if !strings.Contains(result.Text(), `"pending_approval":true`) {
		t.Fatalf("tool should report the batch is waiting on the writer: %s", result.Text())
	}

	after, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("preview must not touch the outline: %d nodes before, %d after", len(before), len(after))
	}

	preview := notif.get("companion.preview")
	if preview == "" {
		t.Fatal("expected a companion.preview notification")
	}
	if !strings.Contains(preview, `"created":9`) {
		t.Fatalf("preview should count the new nodes: %s", preview)
	}
	if !strings.Contains(preview, `"label":"씬 1"`) {
		t.Fatalf("preview should list what would be created: %s", preview)
	}
}

func TestLinettaApplyOpsToolStillAppliesSmallOutlineEdits(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	notif := &fakeNotifier{}
	svc.notify = notif
	userText := "씬 하나만 추가해줘"
	intent := companionIntent{Kind: companionIntentGenericMutation}
	tool := svc.buildApplyOpsTool(projectID, nodeID, HistoryScopeProject, "run-1", userText, intent, "", func() int64 { return 1 })

	result, err := tool.Execute(ctx, outlineApplyParams(t, "씬 추가",
		`[{"op":"create_scene","ref":"s1","label":"씬 2","title":"새 장면"}]`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError || strings.Contains(result.Text(), "pending_approval") {
		t.Fatalf("small edits should apply directly: %s", result.Text())
	}
	if notif.get("companion.preview") != "" {
		t.Fatal("a one-scene edit should not open an approval step")
	}
}

// Half a restructured outline is worse than none.
func TestApplyOpsRollsBackWhenAStructuralOpFails(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	before, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}

	result := svc.ApplyOps(ctx, projectID, nodeID, Proposal{
		Summary: "아웃라인 정리",
		Ops: []Op{
			{Type: "create_outline_node", Ref: "p1", Kind: "container", Label: "1부"},
			{Type: "create_outline_node", Ref: "s1", Kind: "leaf", ParentNodeRef: "p1", Label: "씬 1"},
			{Type: "rename_outline_node", NodeID: "does-not-exist", Label: "3화"},
		},
	}, func() int64 { return 2 })

	if !result.RolledBack {
		t.Fatalf("a failed structural batch should roll back: %+v", result)
	}
	if result.Applied != 0 {
		t.Fatalf("rolled-back batch should report nothing applied: %+v", result)
	}
	if result.UndoBatchID != "" {
		t.Fatalf("nothing changed, so there is nothing to undo: %+v", result)
	}

	after, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("outline should be back to %d nodes, got %d", len(before), len(after))
	}
	beforeIDs := map[string]bool{}
	for _, n := range before {
		beforeIDs[n.ID] = true
	}
	for _, n := range after {
		if !beforeIDs[n.ID] {
			t.Fatalf("rollback left a node the batch created: %+v", n)
		}
	}
}

func TestApplyOpsUndoRestoresTheOutline(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	before, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}

	result := svc.ApplyOps(ctx, projectID, nodeID, Proposal{
		Summary: "1부 아웃라인",
		Ops: []Op{
			{Type: "create_outline_node", Ref: "p1", Kind: "container", Label: "1부"},
			{Type: "create_outline_node", Ref: "s1", Kind: "leaf", ParentNodeRef: "p1", Label: "씬 1"},
		},
	}, func() int64 { return 2 })

	if result.IsError() || result.Applied != 2 {
		t.Fatalf("apply should succeed: %+v", result)
	}
	if result.UndoBatchID == "" {
		t.Fatalf("a structural change should be undoable: %+v", result)
	}
	mid, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}
	if len(mid) != len(before)+2 {
		t.Fatalf("expected 2 new nodes, got %d", len(mid)-len(before))
	}

	if err := svc.UndoApply(ctx, result.UndoBatchID, func() int64 { return 3 }); err != nil {
		t.Fatalf("UndoApply: %v", err)
	}
	after, err := svc.nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("undo should restore %d nodes, got %d", len(before), len(after))
	}

	// The batch is spent: undoing twice must not silently do something else.
	if err := svc.UndoApply(ctx, result.UndoBatchID, func() int64 { return 4 }); err != ErrUndoBatchNotFound {
		t.Fatalf("second undo error = %v, want ErrUndoBatchNotFound", err)
	}
}

func TestApplyOpsSkipsUndoForNonStructuralBatches(t *testing.T) {
	svc, projectID, nodeID := newToolSvc(t)

	result := svc.ApplyOps(context.Background(), projectID, nodeID, Proposal{
		Summary: "작품 개요 정리",
		Ops:     []Op{{Type: "set_outline", Outline: "복수 서사"}},
	}, func() int64 { return 2 })

	if result.IsError() || result.Applied != 1 {
		t.Fatalf("apply should succeed: %+v", result)
	}
	if result.UndoBatchID != "" {
		t.Fatalf("outline-tree undo only covers tree changes: %+v", result)
	}
}
