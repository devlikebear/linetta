package handlers

import (
	"context"
	"encoding/json"
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
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"

	"github.com/devlikebear/linetta/engine/internal/store"
)

type forecastFix struct {
	svc          *forecast.Service
	proposalsSvc *proposals.Service
	projects     *project.Repo
	nodes        *node.Repo
	entities     *entity.Repo
	threads      *thread.Repo
	beats        *beat.Repo
	pID          string
	sceneID      string
	now          Clock
}

func newForecastHandlerFixture(t *testing.T) forecastFix {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	entities := entity.NewRepo(st)
	relationships := relationship.NewRepo(st)
	facts := fact.NewRepo(st)
	directionRepo := direction.NewRepo(st, nodes, projects)

	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first",
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
	so := storyops.New(projects, nodes, threads, beats, entities, relationships).WithFacts(facts)
	proposalsSvc := proposals.New(st, so, entities, threads, beats, facts, nodes, projects, nil).WithForecasts(svc)

	return forecastFix{
		svc: svc, proposalsSvc: proposalsSvc, projects: projects, nodes: nodes,
		entities: entities, threads: threads, beats: beats,
		pID: p.ID, sceneID: sceneID, now: func() int64 { return 2000 },
	}
}

func twoHandlerBranches() []forecast.BranchInput {
	return []forecast.BranchInput{
		{Title: "갈래 1", Premise: "주인공이 맞선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
		{Title: "갈래 2", Premise: "주인공이 물러선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
	}
}

func TestListForecasts_staleAndRefNames(t *testing.T) {
	f := newForecastHandlerFixture(t)
	ctx := context.Background()

	e, err := f.entities.Create(ctx, 1_000, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	branches := twoHandlerBranches()
	branches[0].Decisions = []forecast.CharDecision{{EntityID: e.ID, Decision: "맞선다"}}

	saved, problems, err := f.svc.Save(ctx, f.now(), forecast.SaveInput{
		ProjectID: f.pID, Divergence: "다음 회차 갈래?", Horizon: 3, Branches: branches,
	}, forecast.SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Save: err=%v problems=%+v", err, problems)
	}

	h := ListForecasts(f.svc, f.projects, f.nodes, f.entities, nil)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out struct {
		Forecasts []forecastOut `json:"forecasts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Forecasts) != 1 {
		t.Fatalf("forecasts = %+v", out.Forecasts)
	}
	got := out.Forecasts[0]
	if got.Stale {
		t.Error("fresh forecast reported stale")
	}
	if got.BaseEpisodeLabel == "" {
		t.Error("base_episode_label not resolved")
	}
	if len(got.Branches[0].Decisions) != 1 || got.Branches[0].Decisions[0].EntityName != "하린" {
		t.Errorf("decisions = %+v, want entity_name 하린", got.Branches[0].Decisions)
	}

	// Edit the scene the forecast was drawn from — the list must now
	// report it stale, with an episodes reason.
	doc2, _ := storyops.PlainTextToTiptapDoc("바뀐 문장.")
	if err := f.nodes.UpdateContent(ctx, f.sceneID, doc2, 3); err != nil {
		t.Fatalf("update content: %v", err)
	}
	raw, err = h(ctx, params)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	out.Forecasts = nil
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Forecasts[0].Stale {
		t.Error("forecast should be stale after the scene changed")
	}
	_ = saved
}

func TestSelectForecast_warnsOnStaleButSucceeds(t *testing.T) {
	f := newForecastHandlerFixture(t)
	ctx := context.Background()
	saved, _, err := f.svc.Save(ctx, f.now(), forecast.SaveInput{
		ProjectID: f.pID, Divergence: "질문", Horizon: 3, Branches: twoHandlerBranches(),
	}, forecast.SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, _ := storyops.PlainTextToTiptapDoc("바뀐 문장.")
	if err := f.nodes.UpdateContent(ctx, f.sceneID, doc2, 3); err != nil {
		t.Fatalf("update content: %v", err)
	}

	h := SelectForecast(f.svc, f.now)
	params, _ := json.Marshal(map[string]any{"forecast_id": saved.ID, "branch_id": "b1"})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out struct {
		OK      bool                   `json:"ok"`
		Stale   bool                   `json:"stale"`
		Reasons []forecast.StaleReason `json:"reasons,omitempty"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK {
		t.Error("select of a stale forecast must still succeed")
	}
	if !out.Stale || len(out.Reasons) == 0 {
		t.Errorf("out = %+v, want a stale warning", out)
	}

	sel, found, err := f.svc.Selected(ctx, f.pID)
	if err != nil || !found || sel.SelectedBranch != "b1" {
		t.Fatalf("Selected = %+v found=%v err=%v", sel, found, err)
	}
}

func TestSelectForecast_unknownForecastID(t *testing.T) {
	f := newForecastHandlerFixture(t)
	h := SelectForecast(f.svc, f.now)
	params, _ := json.Marshal(map[string]any{"forecast_id": "no-such", "branch_id": "b1"})
	if _, err := h(context.Background(), params); err == nil {
		t.Fatal("want an error for an unknown forecast id")
	}
}

func TestClearForecastSelectionAndDelete(t *testing.T) {
	f := newForecastHandlerFixture(t)
	ctx := context.Background()
	saved, _, err := f.svc.Save(ctx, f.now(), forecast.SaveInput{
		ProjectID: f.pID, Divergence: "질문", Horizon: 3, Branches: twoHandlerBranches(),
	}, forecast.SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := f.svc.Select(ctx, f.now(), saved.ID, "b1"); err != nil {
		t.Fatalf("Select: %v", err)
	}

	clearH := ClearForecastSelection(f.svc)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID})
	if _, err := clearH(ctx, params); err != nil {
		t.Fatalf("clear handler: %v", err)
	}
	if _, found, err := f.svc.Selected(ctx, f.pID); err != nil || found {
		t.Fatalf("Selected after clear: found=%v err=%v", found, err)
	}

	deleteH := DeleteForecast(f.svc)
	params, _ = json.Marshal(map[string]any{"forecast_id": saved.ID})
	if _, err := deleteH(ctx, params); err != nil {
		t.Fatalf("delete handler: %v", err)
	}
	rows, err := f.svc.List(ctx, f.pID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows = %+v err=%v, want none after delete", rows, err)
	}
}

func TestUndoProposals_revertsStructuralBatch(t *testing.T) {
	f := newForecastHandlerFixture(t)
	ctx := context.Background()
	th, err := f.threads.Create(ctx, thread.NewInput{ProjectID: f.pID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	saved, _, err := f.svc.Save(ctx, f.now(), forecast.SaveInput{
		ProjectID: f.pID, Divergence: "질문", Horizon: 3, Branches: twoHandlerBranches(),
	}, forecast.SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := f.svc.Select(ctx, f.now(), saved.ID, "b1"); err != nil {
		t.Fatalf("Select: %v", err)
	}

	sub, problems, err := f.proposalsSvc.Submit(ctx, f.now(), proposals.SubmitInput{
		ProjectID: f.pID, Purpose: proposals.PurposeForecast, ForecastID: saved.ID,
		Items: []proposals.ItemInput{
			{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: f.sceneID}},
			{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "비트"}},
		},
	}, proposals.SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit: err=%v problems=%+v", err, problems)
	}
	ids := []string{sub.Items[0].ID, sub.Items[1].ID}
	res, err := f.proposalsSvc.Accept(ctx, f.now(), f.pID, ids, nil)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if len(res.UndoBatchIDs) != 1 {
		t.Fatalf("UndoBatchIDs = %+v", res.UndoBatchIDs)
	}
	sceneID := res.Items[0].Result.CreatedID

	h := UndoProposals(f.proposalsSvc, f.now)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID, "batch_id": res.UndoBatchIDs[0]})
	if _, err := h(ctx, params); err != nil {
		t.Fatalf("undo handler: %v", err)
	}
	if _, err := f.nodes.Get(ctx, sceneID); err == nil {
		t.Error("scene still exists after undo")
	}
}

func TestUndoProposals_unknownBatchID(t *testing.T) {
	f := newForecastHandlerFixture(t)
	h := UndoProposals(f.proposalsSvc, f.now)
	params, _ := json.Marshal(map[string]any{"project_id": f.pID, "batch_id": "no-such"})
	if _, err := h(context.Background(), params); err == nil {
		t.Fatal("want an error for an unknown batch id")
	}
}
