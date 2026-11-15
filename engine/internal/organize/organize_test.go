package organize

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

func doc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

var long = strings.Repeat("하린이 창문을 두드렸다. ", 8)

type fixture struct {
	s       *store.Store
	repo    *Repo
	nodes   *node.Repo
	project string
	ep1     string
	scenes  []string // ep1: long, long, short; ep2: long
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	s, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "o.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	nr := node.NewRepo(s)
	p, err := pr.Create(ctx, 1, project.NewInput{Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first", OutlinePreset: project.OutlinePresetWebNovel})
	if err != nil {
		t.Fatal(err)
	}
	seed := *p.LastOpenedNodeID
	f := fixture{s: s, repo: NewRepo(s, nr, pr), nodes: nr, project: p.ID}
	prev := seed
	for i, texts := range [][]string{{long, long + "다른 장면", "짧다"}, {long}} {
		ep, err := nr.CreateSibling(ctx, prev, node.KindContainer, []string{"1화", "2화"}[i], "", 2)
		if err != nil {
			t.Fatal(err)
		}
		prev = ep.ID
		if i == 0 {
			f.ep1 = ep.ID
		}
		for _, text := range texts {
			sc, err := nr.CreateChild(ctx, ep.ID, node.KindLeaf, "씬", "", 3)
			if err != nil {
				t.Fatal(err)
			}
			if err := nr.UpdateContent(ctx, sc.ID, doc(text), 4); err != nil {
				t.Fatal(err)
			}
			f.scenes = append(f.scenes, sc.ID)
		}
	}
	if err := nr.Delete(ctx, seed, 5); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestPlanReadsTheEpisodesChangedScenes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	plan, err := f.repo.Plan(ctx, f.project, f.scenes[1], false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.EpisodeNodeID != f.ep1 || plan.EpisodeLabel != "1화" || len(plan.Scenes) != 2 || plan.Short != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	// The episode container names the same episode.
	if byEpisode, _ := f.repo.Plan(ctx, f.project, f.ep1, false); len(byEpisode.Scenes) != 2 {
		t.Fatalf("plan by episode = %+v", byEpisode)
	}

	if err := f.repo.Record(ctx, f.project, plan.Scenes[0], OutcomeDone, 10); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Record(ctx, f.project, plan.Scenes[1], OutcomeTimeout, 10); err != nil {
		t.Fatal(err)
	}
	plan, _ = f.repo.Plan(ctx, f.project, f.ep1, false)
	if len(plan.Scenes) != 1 || plan.Scenes[0].NodeID != f.scenes[1] || plan.Unchanged != 1 {
		t.Fatalf("after one done and one timeout: %+v", plan)
	}
	if forced, _ := f.repo.Plan(ctx, f.project, f.ep1, true); len(forced.Scenes) != 2 {
		t.Fatalf("forced = %+v", forced)
	}

	// New words are read again; a relink that keeps them is not.
	if err := f.nodes.UpdateContent(ctx, f.scenes[0], doc(long+"새 문장."), 11); err != nil {
		t.Fatal(err)
	}
	if plan, _ = f.repo.Plan(ctx, f.project, f.ep1, false); len(plan.Scenes) != 2 {
		t.Fatalf("after an edit: %+v", plan)
	}
}

func TestRecordKeepsADoneOverALaterUnfinishedPass(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	plan, _ := f.repo.Plan(ctx, f.project, f.ep1, false)
	sc := plan.Scenes[0]
	_ = f.repo.Record(ctx, f.project, sc, OutcomeDone, 10)
	_ = f.repo.Record(ctx, f.project, sc, OutcomeFailed, 11)
	if again, _ := f.repo.Plan(ctx, f.project, f.ep1, false); len(again.Scenes) != 1 {
		t.Fatalf("a failed pass undid a done one: %+v", again)
	}
	if err := f.repo.Record(ctx, f.project, sc, "maybe", 12); err == nil {
		t.Fatal("unknown outcome accepted")
	}
}

func TestPlanMarksScenesTheSettlementAlreadyTook(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	n, _ := f.nodes.Get(ctx, f.scenes[0])
	if _, err := f.s.DB().ExecContext(ctx,
		`INSERT INTO scene_settlements (node_id, project_id, content_version, outcome, settled_at) VALUES (?, ?, ?, 'done', 1)`,
		n.ID, f.project, n.ContentVersion); err != nil {
		t.Fatal(err)
	}
	plan, _ := f.repo.Plan(ctx, f.project, f.ep1, false)
	if !plan.Scenes[0].LedgerSettled || plan.Scenes[1].LedgerSettled {
		t.Fatalf("settled flags = %+v", plan.Scenes)
	}
}

func TestPlanRefusesANodeOutsideTheEpisodes(t *testing.T) {
	f := setup(t)
	if _, err := f.repo.Plan(context.Background(), f.project, "nowhere", false); err != ErrNotInEpisode {
		t.Fatalf("err = %v", err)
	}
}
