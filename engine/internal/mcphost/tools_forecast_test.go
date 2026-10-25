//go:build !mobile

package mcphost

import (
	"context"
	"path/filepath"
	"strings"
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
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type forecastFixture struct {
	d       ToolDeps
	p       project.Project
	sceneID string
	changed int
}

func newForecastFixture(t *testing.T) (context.Context, *forecastFixture) {
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
	directionRepo := direction.NewRepo(st, nodes, projects)

	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sceneID := *p.LastOpenedNodeID
	doc, err := storyops.PlainTextToTiptapDoc("문장.")
	if err != nil {
		t.Fatalf("doc: %v", err)
	}
	if err := nodes.UpdateContent(ctx, sceneID, doc, 2); err != nil {
		t.Fatalf("set scene content: %v", err)
	}

	svc := forecast.New(st, forecast.Deps{
		Projects: projects, Nodes: nodes, Entities: entities, Relationships: relationships,
		Facts: facts, Threads: threads, Beats: beats, Direction: directionRepo,
	})

	f := &forecastFixture{p: p, sceneID: sceneID}
	f.d = ToolDeps{
		Projects:  projects,
		Nodes:     nodes,
		Entities:  entities,
		Beats:     beats,
		Threads:   threads,
		Forecasts: svc,
		Clock:     func() int64 { return 1000 },
		Notify: func(method string, _ any) {
			if method == "mcp.changed" {
				f.changed++
			}
		},
	}
	return ctx, f
}

func twoForecastBranches() []forecast.BranchInput {
	return []forecast.BranchInput{
		{Title: "갈래 1", Premise: "주인공이 맞선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
		{Title: "갈래 2", Premise: "주인공이 물러선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
	}
}

func TestSaveForecast_success(t *testing.T) {
	ctx, f := newForecastFixture(t)
	_, out, err := f.d.saveForecast(ctx, nil, saveForecastInput{
		ProjectID: f.p.ID, Divergence: "다음 회차 갈래?", Horizon: 3, Branches: twoForecastBranches(),
	})
	if err != nil {
		t.Fatalf("saveForecast error: %v", err)
	}
	if out.ForecastID == "" || out.Status != "waiting_for_writer" {
		t.Fatalf("out = %+v", out)
	}
	if len(out.BranchIDs) != 2 || out.BranchIDs[0] != "b1" || out.BranchIDs[1] != "b2" {
		t.Fatalf("branch ids = %+v", out.BranchIDs)
	}
	if out.BaseEpisodeLabel == "" {
		t.Error("base_episode_label not set")
	}
	if f.changed != 1 {
		t.Errorf("mcp.changed fired %d times, want 1", f.changed)
	}
}

func TestSaveForecast_refusesTooManyBranches(t *testing.T) {
	ctx, f := newForecastFixture(t)
	six := make([]forecast.BranchInput, 0, 6)
	for i := 0; i < 6; i++ {
		six = append(six, twoForecastBranches()[0])
	}
	res, _, err := f.d.saveForecast(ctx, nil, saveForecastInput{
		ProjectID: f.p.ID, Divergence: "질문", Horizon: 3, Branches: six,
	})
	if err != nil {
		t.Fatalf("saveForecast error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("want a tool error for 6 branches")
	}
	if f.changed != 0 {
		t.Error("mcp.changed fired on a refused save")
	}
}

func TestSaveForecast_refusesWithoutForecastsDeps(t *testing.T) {
	ctx, f := newForecastFixture(t)
	f.d.Forecasts = nil
	res, _, err := f.d.saveForecast(ctx, nil, saveForecastInput{
		ProjectID: f.p.ID, Divergence: "질문", Horizon: 3, Branches: twoForecastBranches(),
	})
	if err != nil {
		t.Fatalf("saveForecast error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("want a tool error when forecasts are unavailable")
	}
}

func TestSaveForecast_isRegisteredOnlyWhenReviewGroupOn(t *testing.T) {
	names := ToolNames("full", ToolGroups{Review: false})
	for _, n := range names {
		if n == "linetta_save_forecast" {
			t.Fatal("linetta_save_forecast must not be registered when Review is off")
		}
	}
	names = ToolNames("full", ToolGroups{Review: true})
	found := false
	for _, n := range names {
		if n == "linetta_save_forecast" {
			found = true
		}
	}
	if !found {
		t.Fatal("linetta_save_forecast must be registered when Review is on")
	}
}

func TestSaveForecast_isInReviewToolNames(t *testing.T) {
	if !strings.Contains(strings.Join(ReviewToolNames, ","), "linetta_save_forecast") {
		t.Errorf("ReviewToolNames = %v, want linetta_save_forecast", ReviewToolNames)
	}
}
