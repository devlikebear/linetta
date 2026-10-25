package engineapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/forecast"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/proposals"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// revertingSeed is what seedRevertingBatches leaves behind for a test to
// assert against once the real App has started (and run
// proposals.ReconcileReverting) against the same library.db.
type revertingSeed struct {
	itemsA        []string
	sceneA, beatA string
	itemsB        []string
	sceneB, beatB string
}

// seedRevertingBatches opens its OWN store.Store at home's library.db —
// the exact path engineapp.register itself opens — builds the same repos
// Open wires, and leaves two independent purpose=forecast structural
// batches stuck at proposals.StatusReverting for the two different
// reasons ReconcileReverting must tell apart from the outline's own
// facts, with no open App and no live storyops undo window in the
// picture at all (an engine restart always starts storyops with an empty
// window — reconcileBatch doesn't consult it either):
//
//   - batch A: the status write that would have carried it past
//     reverting never ran (as if the process died right after Undo's own
//     step 1) — the node is still in the outline.
//   - batch B: storyops.UndoApply actually ran and put the outline back
//     (the node is gone for real) before the process died, so the
//     finalize write that should have followed it never landed either.
//
// Everything here runs through the real Submit/Accept/UndoApply, not
// hand-built rows — the only hand-written SQL is the one UPDATE neither
// of those exposes: carrying status to reverting while leaving
// result_json and undo_batch_id untouched, exactly what Undo's own step 1
// (proposals/undo.go) does.
func seedRevertingBatches(t *testing.T, home string) revertingSeed {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	entities := entity.NewRepo(st)
	relationships := relationship.NewRepo(st)
	facts := fact.NewRepo(st)
	directionRepo := direction.NewRepo(st, nodes, projects)
	so := storyops.New(projects, nodes, threads, beats, entities, relationships).WithFacts(facts)
	forecasts := forecast.New(st, forecast.Deps{
		Projects: projects, Nodes: nodes, Entities: entities, Relationships: relationships,
		Facts: facts, Threads: threads, Beats: beats, Direction: directionRepo,
	})
	svc := proposals.New(st, so, entities, threads, beats, facts, nodes, projects, nil).WithForecasts(forecasts)

	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "재시작 정리 테스트", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sceneID0 := *p.LastOpenedNodeID
	doc, err := storyops.PlainTextToTiptapDoc("문장.")
	if err != nil {
		t.Fatalf("doc: %v", err)
	}
	if err := nodes.UpdateContent(ctx, sceneID0, doc, 2); err != nil {
		t.Fatalf("set scene content: %v", err)
	}
	th, err := threads.Create(ctx, thread.NewInput{ProjectID: p.ID, Name: "갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	f, problems, err := forecasts.Save(ctx, 1, forecast.SaveInput{
		ProjectID: p.ID, Divergence: "질문", Horizon: 3,
		Branches: []forecast.BranchInput{
			{Title: "갈래 1", Premise: "주인공이 맞선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
			{Title: "갈래 2", Premise: "주인공이 물러선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
		},
	}, forecast.SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("forecast Save: err=%v problems=%+v", err, problems)
	}
	if _, err := forecasts.Select(ctx, 1, p.ID, f.ID, "b1"); err != nil {
		t.Fatalf("forecast Select: %v", err)
	}

	submitAccept := func(ref string) (batchID string, itemIDs []string, sceneID, beatID string) {
		sub, problems, err := svc.Submit(ctx, 1, proposals.SubmitInput{
			ProjectID: p.ID, Purpose: proposals.PurposeForecast, ForecastID: f.ID,
			Items: []proposals.ItemInput{
				{Op: storyops.Op{Type: "create_scene", Ref: ref, Label: "장면", AfterNodeID: sceneID0}},
				{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: ref, Label: "비트"}},
			},
		}, proposals.SourceAgent)
		if err != nil || len(problems) != 0 {
			t.Fatalf("Submit(%s): err=%v problems=%+v", ref, err, problems)
		}
		itemIDs = []string{sub.Items[0].ID, sub.Items[1].ID}
		res, err := svc.Accept(ctx, 1, p.ID, itemIDs, nil)
		if err != nil {
			t.Fatalf("Accept(%s): %v", ref, err)
		}
		if len(res.UndoBatchIDs) != 1 {
			t.Fatalf("Accept(%s) UndoBatchIDs = %+v, want exactly one", ref, res.UndoBatchIDs)
		}
		return res.UndoBatchIDs[0], itemIDs, res.Items[0].Result.CreatedID, res.Items[1].Result.CreatedID
	}

	batchA, itemsA, sceneA, beatA := submitAccept("a1")
	batchB, itemsB, sceneB, beatB := submitAccept("b1")

	moveToReverting := func(itemIDs []string) {
		for _, id := range itemIDs {
			if _, err := st.DB().ExecContext(ctx,
				`UPDATE story_proposal_items SET status = 'reverting', status_at = 1 WHERE id = ?`, id); err != nil {
				t.Fatalf("move %s to reverting: %v", id, err)
			}
		}
	}

	// Batch A: never got past step 1 — node A is left exactly as Accept
	// made it.
	moveToReverting(itemsA)
	_ = batchA

	// Batch B: storyops.UndoApply really ran (node B is actually gone),
	// then — simulating the crash — step 1's own status write is
	// reproduced (step 3's finalize never happened).
	if err := so.UndoApply(ctx, batchB, func() int64 { return 1 }); err != nil {
		t.Fatalf("UndoApply(%s): %v", batchB, err)
	}
	if _, err := nodes.Get(ctx, sceneB); !errors.Is(err, node.ErrNotFound) {
		t.Fatalf("Get sceneB after UndoApply: %v, want ErrNotFound (seed setup is wrong)", err)
	}
	moveToReverting(itemsB)

	return revertingSeed{itemsA: itemsA, sceneA: sceneA, beatA: beatA, itemsB: itemsB, sceneB: sceneB, beatB: beatB}
}

// TestOpen_reconcilesRevertingItemsAtStartup covers task 3's own job: when
// the engine starts, proposals.ReconcileReverting must run once, before
// the RPC server takes a single request, and resolve every item an
// earlier crash left stuck at reverting — purely from the outline's own
// facts, exactly like proposals.reconcileBatch's own unit tests, but
// through the real engineapp.Open path against the real library.db file.
func TestOpen_reconcilesRevertingItemsAtStartup(t *testing.T) {
	home := t.TempDir()
	seed := seedRevertingBatches(t, home)

	app := openTestApp(t, home)
	if err := app.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()
	verify, err := store.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open (verify): %v", err)
	}
	defer verify.Close()
	nodes := node.NewRepo(verify)
	beats := beat.NewRepo(verify)

	// Batch A: the node is still there, so reconcile must have restored
	// both items to accepted+original result+batch id.
	for _, id := range seed.itemsA {
		var status, undoBatchID string
		if err := verify.DB().QueryRowContext(ctx,
			`SELECT status, COALESCE(undo_batch_id, '') FROM story_proposal_items WHERE id = ?`, id,
		).Scan(&status, &undoBatchID); err != nil {
			t.Fatalf("query item %s: %v", id, err)
		}
		if status != proposals.StatusAccepted {
			t.Errorf("batch A item %s status = %q, want accepted", id, status)
		}
		if undoBatchID == "" {
			t.Errorf("batch A item %s undo_batch_id is empty, want preserved", id)
		}
	}
	if _, err := nodes.Get(ctx, seed.sceneA); err != nil {
		t.Errorf("Get sceneA: %v, want it to still exist", err)
	}
	if _, err := beats.Get(ctx, seed.beatA); err != nil {
		t.Errorf("Get beatA: %v, want it to still exist (reconcile restores to accepted, never deletes)", err)
	}

	// Batch B: the node is gone, so reconcile must have finalized both
	// items to pending with result and undo_batch_id cleared, and
	// deleted the now-orphaned beat.
	for _, id := range seed.itemsB {
		var status, undoBatchID, resultJSON string
		if err := verify.DB().QueryRowContext(ctx,
			`SELECT status, COALESCE(undo_batch_id, ''), result_json FROM story_proposal_items WHERE id = ?`, id,
		).Scan(&status, &undoBatchID, &resultJSON); err != nil {
			t.Fatalf("query item %s: %v", id, err)
		}
		if status != proposals.StatusPending {
			t.Errorf("batch B item %s status = %q, want pending", id, status)
		}
		if undoBatchID != "" {
			t.Errorf("batch B item %s undo_batch_id = %q, want cleared", id, undoBatchID)
		}
		if resultJSON != "{}" {
			t.Errorf("batch B item %s result_json = %q, want cleared", id, resultJSON)
		}
	}
	if _, err := beats.Get(ctx, seed.beatB); !errors.Is(err, beat.ErrNotFound) {
		t.Errorf("Get beatB: %v, want ErrNotFound (reconcile must clean it up when finalizing to pending)", err)
	}
}
