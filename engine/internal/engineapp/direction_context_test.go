package engineapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/forecast"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// Review finding (round 4, 2026-10-25): directionBriefSource.PlannedBranchFor
// used to swallow a failure from forecast.Service.Stale into reasons = nil,
// which reads to the brief as "checked, and it's fresh" — the exact
// anti-pattern already fixed in forecast.go's SelectForecast (f7) for the
// sibling RPC path. This white-box test builds the real
// directionBriefSource{repo, forecast} this package wires in Open (see
// engineapp.go's WithDirectionSource call), saves and selects a real
// forecast, then breaks forecast.Service.Stale's own read path without
// touching anything Selected/Select themselves read — the same technique
// forecast_test.go's TestSelectForecast_surfacesAStalenessCheckFailure...
// uses for the RPC handler's own version of this bug.
func TestPlannedBranchFor_propagatesAStaleCheckFailureRatherThanReportingFresh(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	entities := entity.NewRepo(st)
	relationships := relationship.NewRepo(st)
	facts := fact.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	directionRepo := direction.NewRepo(st, nodes, projects)

	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title: "예측 테스트", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	forecastSvc := forecast.New(st, forecast.Deps{
		Projects: projects, Nodes: nodes, Entities: entities, Relationships: relationships,
		Facts: facts, Threads: threads, Beats: beats, Direction: directionRepo,
	})
	source := directionBriefSource{repo: directionRepo, forecast: forecastSvc}

	branches := []forecast.BranchInput{
		{Title: "갈래 1", Premise: "주인공이 맞선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
		{Title: "갈래 2", Premise: "주인공이 물러선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
	}
	f, problems, err := forecastSvc.Save(ctx, 10_000, forecast.SaveInput{
		ProjectID: p.ID, Divergence: "다음 회차 갈래?", Horizon: 3, Branches: branches,
	}, forecast.SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Save: err=%v problems=%+v", err, problems)
	}
	if _, err := forecastSvc.Select(ctx, 10_001, p.ID, f.ID, "b1"); err != nil {
		t.Fatalf("Select: %v", err)
	}

	// Sanity: before anything is broken, the planned branch is readable.
	plan, found, err := source.PlannedBranchFor(ctx, p.ID)
	if err != nil || !found || plan.Title != "갈래 1" {
		t.Fatalf("PlannedBranchFor before break = %+v found=%v err=%v, want the selected branch", plan, found, err)
	}

	// Break Stale's own read path (Inputs -> direction.Repo.ListSceneIntents)
	// without touching anything Selected/Select themselves read (only the
	// forecasts table) — so Selected still succeeds and only the staleness
	// check fails.
	if _, err := st.DB().ExecContext(ctx, "DROP TABLE scene_intents"); err != nil {
		t.Fatalf("drop scene_intents: %v", err)
	}

	plan, found, err = source.PlannedBranchFor(ctx, p.ID)
	if err == nil {
		t.Fatalf("want an error when the staleness check itself fails, got found=%v plan=%+v (Stale=%v)", found, plan, plan.Stale)
	}
	if found {
		t.Errorf("found = %v, want false — an error must not also claim the section is present", found)
	}
}
