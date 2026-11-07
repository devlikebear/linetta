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

const proseDoc = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]}]}`

// Four episodes of a web novel, one scene each; the first is written. The
// writer chose a branch whose only beat falls on the third episode.
func TestPlanLookup_readsThePlanTheIntentAndTheChosenBranch(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	directionRepo := direction.NewRepo(st, nodes, projects)
	forecastSvc := forecast.New(st, forecast.Deps{
		Projects: projects, Nodes: nodes, Entities: entity.NewRepo(st), Relationships: relationship.NewRepo(st),
		Facts: fact.NewRepo(st), Threads: thread.NewRepo(st), Beats: beat.NewRepo(st), Direction: directionRepo,
	})
	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title: "문턱", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "first", OutlinePreset: project.OutlinePresetWebNovel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM nodes WHERE project_id = ?`, p.ID); err != nil {
		t.Fatal(err)
	}
	var episodes, scenes []string
	for i, label := range []string{"1화", "2화", "3화", "4화"} {
		ep, err := nodes.CreateRoot(ctx, p.ID, node.KindContainer, label, "", int64(2_000+i))
		if err != nil {
			t.Fatal(err)
		}
		sc, err := nodes.CreateChild(ctx, ep.ID, node.KindLeaf, "씬 1", "", int64(2_000+i))
		if err != nil {
			t.Fatal(err)
		}
		episodes, scenes = append(episodes, ep.ID), append(scenes, sc.ID)
	}
	if err := nodes.UpdateContent(ctx, scenes[0], proseDoc, 3_000); err != nil {
		t.Fatal(err)
	}
	f, problems, err := forecastSvc.Save(ctx, 4_000, forecast.SaveInput{
		ProjectID: p.ID, Divergence: "다음은?", Horizon: 3,
		Branches: []forecast.BranchInput{
			{Title: "갈래", Premise: "맞선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 2, Summary: "결투"}}},
			{Title: "다른 갈래", Premise: "물러선다.", Beats: []forecast.BranchBeat{{EpisodeOffset: 3, Summary: "도망"}}},
		},
	}, forecast.SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Save: %v %+v", err, problems)
	}
	if _, err := forecastSvc.Select(ctx, 4_001, p.ID, f.ID, "b1"); err != nil {
		t.Fatal(err)
	}
	lookup := planLookup{nodes: nodes, projects: projects, direction: directionRepo, forecasts: forecastSvc}
	state := func(id string) (bool, bool) {
		t.Helper()
		empty, planned, err := lookup.PlanState(ctx, id)
		if err != nil {
			t.Fatalf("PlanState(%s): %v", id, err)
		}
		return empty, planned
	}

	if empty, _ := state(scenes[0]); empty {
		t.Error("the written scene reads as empty")
	}
	if empty, _ := state(episodes[1]); empty {
		t.Error("an episode container reads as an empty scene")
	}
	if empty, planned := state(scenes[1]); !empty || planned {
		t.Errorf("2화 = empty %v planned %v, want an empty scene with no plan", empty, planned)
	}
	if _, planned := state(scenes[2]); !planned {
		t.Error("3화 has the chosen branch's beat and still reads unplanned")
	}
	if _, planned := state(scenes[3]); planned {
		t.Error("4화 has no beat and reads planned")
	}

	if _, err := directionRepo.SetSceneIntent(ctx, 5_000, direction.SceneIntent{NodeID: scenes[1], ProjectID: p.ID, Goal: "형을 만난다"}); err != nil {
		t.Fatal(err)
	}
	if _, planned := state(scenes[1]); !planned {
		t.Error("a scene intent is not read as a plan")
	}
	if err := nodes.SetPlan(ctx, episodes[3], "마무리"); err != nil {
		t.Fatal(err)
	}
	if _, planned := state(scenes[3]); !planned {
		t.Error("the episode's own plan is not read as its scene's")
	}
	if empty, planned := state("no-such-node"); empty || planned {
		t.Error("a missing node reads as an empty scene")
	}
}
