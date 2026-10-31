//go:build !mobile

package mcphost

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
	"github.com/devlikebear/linetta/engine/internal/proposals"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type proposalsFixture struct {
	d       ToolDeps
	p       project.Project
	changed int
}

func newProposalsFixture(t *testing.T) (context.Context, *proposalsFixture) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	entities := entity.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	facts := fact.NewRepo(st)
	relationships := relationship.NewRepo(st)

	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	so := storyops.New(projects, nodes, threads, beats, entities, relationships).WithFacts(facts)
	svc := proposals.New(st, so, entities, threads, beats, facts, nodes, projects, nil)

	f := &proposalsFixture{p: p}
	f.d = ToolDeps{
		Projects:  projects,
		Nodes:     nodes,
		Entities:  entities,
		Beats:     beats,
		Threads:   threads,
		Proposals: svc,
		Clock:     func() int64 { return 1000 },
		Notify: func(method string, _ any) {
			if method == "mcp.changed" {
				f.changed++
			}
		},
	}
	return ctx, f
}

func TestProposeStoryOps_success(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	_, out, err := f.d.proposeStoryOps(ctx, nil, proposeStoryOpsInput{
		ProjectID: f.p.ID,
		Items:     []proposals.ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린"}}},
	})
	if err != nil {
		t.Fatalf("proposeStoryOps error: %v", err)
	}
	if out.ProposalID == "" || out.Status != "waiting_for_writer" {
		t.Fatalf("out = %+v", out)
	}
	if f.changed != 1 {
		t.Errorf("mcp.changed fired %d times, want 1", f.changed)
	}
}

func TestProposeStoryOps_refusesDisallowedOp(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	res, _, err := f.d.proposeStoryOps(ctx, nil, proposeStoryOpsInput{
		ProjectID: f.p.ID,
		Items:     []proposals.ItemInput{{Op: storyops.Op{Type: "set_scene_text", NodeID: "x", Text: "hi"}}},
	})
	if err != nil {
		t.Fatalf("proposeStoryOps error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("want a tool error for a disallowed op")
	}
	if !strings.Contains(firstText(res), "linetta_apply_story_ops") {
		t.Errorf("message = %q, want guidance toward linetta_apply_story_ops", firstText(res))
	}
	if f.changed != 0 {
		t.Errorf("mcp.changed fired on a refused submission")
	}
}

func TestProposeStoryOps_refusesWithoutProposalsDeps(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	f.d.Proposals = nil
	res, _, err := f.d.proposeStoryOps(ctx, nil, proposeStoryOpsInput{
		ProjectID: f.p.ID,
		Items:     []proposals.ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "x"}}},
	})
	if err != nil {
		t.Fatalf("proposeStoryOps error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("want a tool error when Proposals is nil")
	}
}

func TestProposeStoryOps_includesProgressForImportExtraction(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	leaf := *f.p.LastOpenedNodeID
	_, out, err := f.d.proposeStoryOps(ctx, nil, proposeStoryOpsInput{
		ProjectID: f.p.ID,
		Purpose:   proposals.PurposeImportExtraction,
		Items: []proposals.ItemInput{{
			Op:          storyops.Op{Type: "create_entity", Name: "하린"},
			FromNodeIDs: []string{leaf},
		}},
	})
	if err != nil {
		t.Fatalf("proposeStoryOps error: %v", err)
	}
	if out.Progress == nil {
		t.Fatal("want progress set for purpose=import_extraction")
	}
	if out.Progress.Covered != 1 || out.Progress.Total != 1 {
		t.Errorf("progress = %+v, want covered=1 total=1", out.Progress)
	}
}

func TestProposeStoryOps_notRegisteredWhenReviewGroupOff(t *testing.T) {
	names := ToolNames("full", ToolGroups{Review: false})
	for _, n := range names {
		if n == "linetta_propose_story_ops" {
			t.Fatal("linetta_propose_story_ops must not be registered when Review is off")
		}
	}
	names = ToolNames("full", ToolGroups{Review: true})
	found := false
	for _, n := range names {
		if n == "linetta_propose_story_ops" {
			found = true
		}
	}
	if !found {
		t.Fatal("linetta_propose_story_ops must be registered when Review is on")
	}
}

func (f *proposalsFixture) submit(t *testing.T, ctx context.Context, items ...proposals.ItemInput) proposeStoryOpsOutput {
	t.Helper()
	res, out, err := f.d.proposeStoryOps(ctx, nil, proposeStoryOpsInput{ProjectID: f.p.ID, Items: items})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("propose: %v %+v", err, res)
	}
	return out
}

func TestListProposals_showsOpenItemsWithTheirIDs(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	sub := f.submit(t, ctx,
		proposals.ItemInput{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린", Kind: "character"}},
		proposals.ItemInput{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		proposals.ItemInput{Op: storyops.Op{Type: "create_entity", Name: "유나", Kind: "character"}},
	)
	if _, err := f.d.Proposals.Reject(ctx, 1, f.p.ID, []string{sub.Items[2].ID}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	res, out, err := f.d.listProposals(ctx, nil, listProposalsInput{ProjectID: f.p.ID})
	if err != nil || res != nil {
		t.Fatalf("list: %v %+v", err, res)
	}
	if out.Total != 2 || len(out.Items) != 2 || len(out.Batches) != 1 || out.Batches[0].Open != 2 {
		t.Fatalf("out = %+v, want the two open items of one batch", out)
	}
	if !strings.Contains(out.Items[1].Summary, "하린") || len(out.Items[1].DependsOn) == 0 {
		t.Fatalf("relationship row = %+v", out.Items[1])
	}

	_, page, err := f.d.listProposals(ctx, nil, listProposalsInput{ProjectID: f.p.ID, Limit: 1, Offset: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != sub.Items[1].ID {
		t.Fatalf("page = %+v, err = %v", page, err)
	}
}

func TestAcceptProposals_byBatchLinksAnExistingNameAndItsRelationship(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	existing, err := f.d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: f.p.ID, Kind: "character", Name: "백설화"})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	sub := f.submit(t, ctx,
		proposals.ItemInput{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "백설화", Kind: "character"}},
		proposals.ItemInput{Op: storyops.Op{Type: "create_entity", Ref: "e2", Name: "한서린", Kind: "character"}},
		proposals.ItemInput{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e2", Label: "동료"}},
	)
	f.changed = 0
	res, out, err := f.d.acceptProposals(ctx, nil, decideProposalsInput{ProjectID: f.p.ID, ProposalIDs: []string{sub.ProposalID}})
	if err != nil || res != nil {
		t.Fatalf("accept: %v %+v", err, res)
	}
	if out.Accepted != 3 || out.Failed != 0 {
		t.Fatalf("out = %+v", out)
	}
	if !out.Items[0].LinkedExisting || out.Items[0].CreatedID != existing.ID {
		t.Fatalf("first item = %+v, want it linked to %s", out.Items[0], existing.ID)
	}
	if f.changed != 1 {
		t.Fatalf("mcp.changed fired %d times, want 1", f.changed)
	}
}

func TestRejectProposals_clearsFailedItemsAndCountsTheRest(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	if _, err := f.d.Entities.Create(ctx, 1, entity.NewInput{ProjectID: f.p.ID, Kind: "place", Name: "청운"}); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	sub := f.submit(t, ctx,
		proposals.ItemInput{Op: storyops.Op{Type: "create_entity", Name: "청운", Kind: "character"}},
		proposals.ItemInput{Op: storyops.Op{Type: "create_entity", Name: "하린", Kind: "character"}},
	)
	_, accepted, err := f.d.acceptProposals(ctx, nil, decideProposalsInput{
		ProjectID: f.p.ID, ItemIDs: []string{sub.Items[0].ID, sub.Items[1].ID},
	})
	if err != nil || accepted.Failed != 1 || accepted.Accepted != 1 {
		t.Fatalf("accepted = %+v, err = %v", accepted, err)
	}
	_, out, err := f.d.rejectProposals(ctx, nil, decideProposalsInput{
		ProjectID: f.p.ID, ItemIDs: []string{sub.Items[0].ID, sub.Items[1].ID, "nope"},
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if out.Rejected != 1 || out.Skipped != 2 {
		t.Fatalf("out = %+v, want the failed item rejected and the accepted and unknown ones skipped", out)
	}
	_, left, err := f.d.listProposals(ctx, nil, listProposalsInput{ProjectID: f.p.ID})
	if err != nil || left.Total != 0 {
		t.Fatalf("left = %+v, err = %v", left, err)
	}
}

func TestDecideProposals_refusesWithoutATargetOrAnUnknownBatch(t *testing.T) {
	ctx, f := newProposalsFixture(t)
	for _, in := range []decideProposalsInput{
		{ProjectID: f.p.ID},
		{ProjectID: f.p.ID, ProposalIDs: []string{"nope"}},
	} {
		if res, _, err := f.d.acceptProposals(ctx, nil, in); err != nil || res == nil || !res.IsError {
			t.Errorf("accept %+v: res=%+v err=%v, want a tool error", in, res, err)
		}
		if res, _, err := f.d.rejectProposals(ctx, nil, in); err != nil || res == nil || !res.IsError {
			t.Errorf("reject %+v: res=%+v err=%v, want a tool error", in, res, err)
		}
	}
}

func TestInboxTools_followTheReviewGroupAndMode(t *testing.T) {
	writes := []string{"linetta_accept_proposals", "linetta_reject_proposals"}
	served := func(mode string, groups ToolGroups) map[string]bool {
		out := map[string]bool{}
		for _, n := range registeredToolNamesOf(t, ToolDeps{}, mode, groups) {
			out[n] = true
		}
		return out
	}
	readOnly := served(settings.MCPModeReadOnly, AllToolGroups())
	for _, n := range writes {
		if readOnly[n] {
			t.Errorf("%s must not be served read-only", n)
		}
	}
	if !readOnly["linetta_list_proposals"] {
		t.Error("linetta_list_proposals is a read and must be served read-only")
	}
	off := served(settings.MCPModeFull, ToolGroups{})
	for _, n := range append(writes, "linetta_list_proposals") {
		if off[n] {
			t.Errorf("%s must leave with the Review group", n)
		}
	}
}
