package proposals

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// mustSubmitForecast submits a purpose=forecast batch against a freshly
// saved-and-selected forecast, failing the test on any problem.
func mustSubmitForecast(t *testing.T, env *testEnv, items []ItemInput) (SubmitResult, string) {
	t.Helper()
	f := env.savedSelectedForecast(t)
	res, problems, err := env.svc.Submit(context.Background(), now(), SubmitInput{
		ProjectID: env.project.ID, Purpose: PurposeForecast, ForecastID: f.ID, Items: items,
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit() problems=%+v err=%v", problems, err)
	}
	return res, f.ID
}

func TestAccept_forecast_batchesStructuralOpsInOneApplyCall(t *testing.T) {
	env := newTestEnv(t)
	th, err := env.threads.Create(context.Background(), thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "create_scene", Ref: "s2", Label: "맞선 다음", AfterNodeRef: "s1"}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s2", Label: "맞선 비트"}},
	})
	ids := []string{sub.Items[0].ID, sub.Items[1].ID, sub.Items[2].ID}

	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, ids, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	for _, it := range res.Items {
		if it.Status != StatusAccepted {
			t.Fatalf("item %+v not accepted", it)
		}
		if it.Result.CreatedID == "" {
			t.Fatalf("item %+v has no created id", it)
		}
	}
	if len(res.UndoBatchIDs) != 1 || res.UndoBatchIDs[0] == "" {
		t.Fatalf("UndoBatchIDs = %+v, want exactly one batch id", res.UndoBatchIDs)
	}
	// All three items — the two structural ones AND the add_beat that
	// depended on s2 — must share the same undo batch.
	for _, it := range res.Items {
		if it.UndoBatchID != res.UndoBatchIDs[0] {
			t.Errorf("item %+v UndoBatchID = %q, want %q", it, it.UndoBatchID, res.UndoBatchIDs[0])
		}
	}

	nodes, err := env.nodes.ListByProject(context.Background(), env.project.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	foundS1, foundS2 := false, false
	for _, n := range nodes {
		if n.ID == res.Items[0].Result.CreatedID {
			foundS1 = true
		}
		if n.ID == res.Items[1].Result.CreatedID {
			foundS2 = true
		}
	}
	if !foundS1 || !foundS2 {
		t.Fatalf("created scenes missing from the outline: s1=%v s2=%v", foundS1, foundS2)
	}

	beats, err := env.beats.ListByThread(context.Background(), th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	if len(beats) != 1 || beats[0].ID != res.Items[2].Result.CreatedID {
		t.Fatalf("beats = %+v, want exactly the accepted one", beats)
	}
}

func TestAccept_forecast_wholeStructuralBatchFailsTogether(t *testing.T) {
	env := newTestEnv(t)
	// env.sceneID is a real leaf node — Submit only confirms a
	// parent_node_id exists in this work (ownership), not that it is a
	// container; storyops' own apply-time check ("parent must be a
	// container") is what must fail here, after s1 already applied
	// successfully within the SAME batch — proving the whole batch, not
	// just the second op, rolls back.
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "create_outline_node", Ref: "bad", Label: "잘못된 부모",
			ParentNodeID: env.sceneID}},
	})
	ids := []string{sub.Items[0].ID, sub.Items[1].ID}

	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, ids, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	for _, it := range res.Items {
		if it.Status != StatusFailed {
			t.Errorf("item %+v = %q, want the whole batch to fail together", it, it.Status)
		}
	}
	if len(res.UndoBatchIDs) != 0 {
		t.Errorf("UndoBatchIDs = %+v, want none when the batch failed", res.UndoBatchIDs)
	}

	// The scene from the failed batch must not have been left behind —
	// storyops' own structural rollback.
	nodes, err := env.nodes.ListByProject(context.Background(), env.project.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	for _, n := range nodes {
		if n.Label == "맞선 장면" {
			t.Fatalf("scene from the failed batch was not rolled back: %+v", n)
		}
	}
}

func TestAccept_forecast_addBeatStillAppliesOneAtATimeOutsideTheStructuralBatch(t *testing.T) {
	// An add_beat targeting a REAL existing scene (not one this batch
	// created) is not part of the structural batch and carries no undo
	// batch id of its own.
	env := newTestEnv(t)
	th, err := env.threads.Create(context.Background(), thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeID: env.sceneID, Label: "기존 장면 비트"}},
	})
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, []string{sub.Items[0].ID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Status != StatusAccepted {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Items[0].UndoBatchID != "" {
		t.Errorf("UndoBatchID = %q, want empty — this beat did not depend on a structural batch", res.Items[0].UndoBatchID)
	}
	if len(res.UndoBatchIDs) != 0 {
		t.Errorf("UndoBatchIDs = %+v, want none", res.UndoBatchIDs)
	}
}

func TestUndo_revertsStructuralBatchAndDependentBeats(t *testing.T) {
	env := newTestEnv(t)
	th, err := env.threads.Create(context.Background(), thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "맞선 비트"}},
	})
	ids := []string{sub.Items[0].ID, sub.Items[1].ID}
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, ids, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if len(res.UndoBatchIDs) != 1 {
		t.Fatalf("UndoBatchIDs = %+v, want exactly one", res.UndoBatchIDs)
	}
	batchID := res.UndoBatchIDs[0]
	sceneID := res.Items[0].Result.CreatedID
	beatID := res.Items[1].Result.CreatedID

	if err := env.svc.Undo(context.Background(), now(), env.project.ID, batchID); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if _, err := env.nodes.Get(context.Background(), sceneID); err == nil {
		t.Error("scene still exists after Undo")
	} else if err != node.ErrNotFound {
		t.Errorf("Get scene after Undo: %v, want ErrNotFound", err)
	}

	beats, err := env.beats.ListByThread(context.Background(), th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	for _, b := range beats {
		if b.ID == beatID {
			t.Fatalf("beat %q still exists after Undo", beatID)
		}
	}

	// Both items are retryable again.
	for _, id := range ids {
		it, err := env.svc.getItem(context.Background(), env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusPending {
			t.Errorf("item %s status = %q, want pending", id, it.Status)
		}
		if it.UndoBatchID != "" {
			t.Errorf("item %s UndoBatchID = %q, want cleared", id, it.UndoBatchID)
		}
	}

	// A second Undo on the same (now-gone) batch id is a clean "not found",
	// not a crash — the writer may click the revert button twice.
	if err := env.svc.Undo(context.Background(), now(), env.project.ID, batchID); err != ErrUndoBatchNotFound {
		t.Errorf("second Undo = %v, want ErrUndoBatchNotFound", err)
	}
}

func TestUndo_unknownBatchID(t *testing.T) {
	env := newTestEnv(t)
	if err := env.svc.Undo(context.Background(), now(), env.project.ID, "no-such-batch"); err != ErrUndoBatchNotFound {
		t.Errorf("Undo = %v, want ErrUndoBatchNotFound", err)
	}
}

// TestAccept_forecast_addBeatAcceptedInALaterCallStillRidesTheStructuralUndoBatch
// is review finding f3: a writer who accepts the structural item (the
// scene) alone first, then accepts the dependent add_beat item in a
// SEPARATE, later Accept call, must still have that add_beat item tagged
// with the structural item's own undo batch — so proposals.undo on that
// batch deletes the beat too, instead of leaving it orphaned with the
// item stuck at "accepted".
func TestAccept_forecast_addBeatAcceptedInALaterCallStillRidesTheStructuralUndoBatch(t *testing.T) {
	env := newTestEnv(t)
	th, err := env.threads.Create(context.Background(), thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "맞선 비트"}},
	})
	sceneItemID, beatItemID := sub.Items[0].ID, sub.Items[1].ID

	// Call 1: accept the structural item alone.
	res1, err := env.svc.Accept(context.Background(), now(), env.project.ID, []string{sceneItemID}, nil)
	if err != nil {
		t.Fatalf("Accept call 1: %v", err)
	}
	if len(res1.UndoBatchIDs) != 1 || res1.UndoBatchIDs[0] == "" {
		t.Fatalf("UndoBatchIDs after call 1 = %+v, want exactly one batch id", res1.UndoBatchIDs)
	}
	batchID := res1.UndoBatchIDs[0]

	// Call 2: accept the dependent add_beat item in a SEPARATE call.
	res2, err := env.svc.Accept(context.Background(), now()+1, env.project.ID, []string{beatItemID}, nil)
	if err != nil {
		t.Fatalf("Accept call 2: %v", err)
	}
	if len(res2.Items) != 1 || res2.Items[0].Status != StatusAccepted {
		t.Fatalf("call 2 items = %+v", res2.Items)
	}
	if res2.Items[0].UndoBatchID != batchID {
		t.Fatalf("add_beat item's UndoBatchID = %q, want %q (the structural item's own batch, accepted earlier)",
			res2.Items[0].UndoBatchID, batchID)
	}
	beatID := res2.Items[0].Result.CreatedID
	if beatID == "" {
		t.Fatal("add_beat item has no created id")
	}

	// Undo the structural batch: both the scene AND the beat created in
	// the LATER call must go, and the add_beat item must be retryable.
	if err := env.svc.Undo(context.Background(), now()+2, env.project.ID, batchID); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if _, err := env.nodes.Get(context.Background(), res1.Items[0].Result.CreatedID); err == nil {
		t.Error("scene still exists after Undo")
	}
	beats, err := env.beats.ListByThread(context.Background(), th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	for _, b := range beats {
		if b.ID == beatID {
			t.Fatalf("beat %q (accepted in a later call) still exists after undoing its structural batch", beatID)
		}
	}
	it, err := env.svc.getItem(context.Background(), env.svc.store.DB(), beatItemID)
	if err != nil {
		t.Fatalf("getItem: %v", err)
	}
	if it.Status != StatusPending {
		t.Errorf("add_beat item status = %q, want pending after Undo", it.Status)
	}
	if it.UndoBatchID != "" {
		t.Errorf("add_beat item UndoBatchID = %q, want cleared after Undo", it.UndoBatchID)
	}
}

// TestAccept_forecast_editDroppingTheNodeRefDoesNotInheritAnUnrelatedUndoBatch
// is review finding f1: an add_beat item's undo_batch_id used to be read
// off its STORED depends_on (Submit's reading of the original op), not the
// ref the actually-applied (possibly edited) op still carries. An edit that
// drops node_ref for a direct node_id pointing at a totally unrelated,
// already-existing scene must not tag the resulting beat with the
// structural sibling's batch — undoing that batch must not delete a beat
// that has nothing to do with it.
func TestAccept_forecast_editDroppingTheNodeRefDoesNotInheritAnUnrelatedUndoBatch(t *testing.T) {
	env := newTestEnv(t)
	th, err := env.threads.Create(context.Background(), thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "맞선 비트"}},
	})
	sceneItemID, beatItemID := sub.Items[0].ID, sub.Items[1].ID

	// Accept the structural item alone — it gets its own undo batch.
	res1, err := env.svc.Accept(context.Background(), now(), env.project.ID, []string{sceneItemID}, nil)
	if err != nil {
		t.Fatalf("Accept call 1: %v", err)
	}
	if len(res1.UndoBatchIDs) != 1 || res1.UndoBatchIDs[0] == "" {
		t.Fatalf("UndoBatchIDs after call 1 = %+v, want exactly one batch id", res1.UndoBatchIDs)
	}
	batchID := res1.UndoBatchIDs[0]

	// Accept the add_beat item with an edit that drops node_ref entirely,
	// pointing it instead at env.sceneID — a real, already-existing scene
	// that has nothing to do with the structural batch above.
	edits := map[string]storyops.Op{
		beatItemID: {Type: "add_beat", ThreadID: th.ID, NodeID: env.sceneID, Label: "무관한 장면의 비트"},
	}
	res2, err := env.svc.Accept(context.Background(), now()+1, env.project.ID, []string{beatItemID}, edits)
	if err != nil {
		t.Fatalf("Accept call 2: %v", err)
	}
	if len(res2.Items) != 1 || res2.Items[0].Status != StatusAccepted {
		t.Fatalf("call 2 items = %+v", res2.Items)
	}
	if res2.Items[0].UndoBatchID != "" {
		t.Fatalf("add_beat item's UndoBatchID = %q, want empty — the edit no longer references the structural batch",
			res2.Items[0].UndoBatchID)
	}

	// The edit carries no ref of its own (same as this package's own
	// TestAccept_editDroppingARefDependencyIsNotBlockedByTheStaleDependency),
	// so the applied beat's own id is identified by its label rather than
	// Result.CreatedID — resultIDFor's ref-tracking fallback is specific to
	// create_entity/create_thread edits and plays no part here.
	beforeUndo, err := env.beats.ListByThread(context.Background(), th.ID)
	if err != nil {
		t.Fatalf("ListByThread before Undo: %v", err)
	}
	if len(beforeUndo) != 1 || beforeUndo[0].Label != "무관한 장면의 비트" {
		t.Fatalf("beats before Undo = %+v, want exactly the accepted one", beforeUndo)
	}

	// Undoing the UNRELATED structural batch must leave this beat alone.
	if err := env.svc.Undo(context.Background(), now()+2, env.project.ID, batchID); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	afterUndo, err := env.beats.ListByThread(context.Background(), th.ID)
	if err != nil {
		t.Fatalf("ListByThread after Undo: %v", err)
	}
	if len(afterUndo) != 1 || afterUndo[0].Label != "무관한 장면의 비트" {
		t.Fatalf("beats after Undo = %+v, want the unrelated beat still there — it never depended on the undone batch", afterUndo)
	}
}

// TestAccept_forecast_addBeatWithoutItsStructuralSiblingRefusesWithADependencyError
// is review finding f3: accepting a forecast add_beat item without its
// structural sibling (the scene it names by node_ref) — reachable from
// ProposalInbox, since unchecking an auto-selected dependency does not also
// uncheck the dependent — must fail with the same friendly "depends on …,
// which has not been accepted" error every other dependency gate in this
// file gives, not storyops' raw "scene ref/id not found".
func TestAccept_forecast_addBeatWithoutItsStructuralSiblingRefusesWithADependencyError(t *testing.T) {
	env := newTestEnv(t)
	th, err := env.threads.Create(context.Background(), thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "맞선 비트"}},
	})
	beatItemID := sub.Items[1].ID

	// Accept ONLY the add_beat item — its structural sibling (the scene it
	// depends on) is deliberately left pending/unrequested.
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, []string{beatItemID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Status != StatusFailed {
		t.Fatalf("items = %+v, want the add_beat item refused", res.Items)
	}
	msg := res.Items[0].Result.Error
	if !strings.Contains(msg, "depends on") || !strings.Contains(msg, "has not been accepted") {
		t.Errorf("error = %q, want the friendly dependency-missing message, not a raw storyops failure", msg)
	}
}
