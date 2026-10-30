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
