package contextualedit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type contextualFixture struct {
	ctx      context.Context
	nodes    *node.Repo
	entities *entity.Repo
	facts    *fact.Repo
	rels     *relationship.Repo
	svc      *Service
	pid      string
	sceneID  string
}

func newContextualFixture(t *testing.T) contextualFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 1_000, project.NewInput{
		Title: "T", Genres: []string{"판타지"}, LengthTarget: "series", DefaultPOV: "third_limited",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	nodes := node.NewRepo(st)
	entities := entity.NewRepo(st)
	facts := fact.NewRepo(st)
	rels := relationship.NewRepo(st)
	manuscript := manuscriptedit.NewService(nodes, snapshot.NewRepo(st))
	return contextualFixture{
		ctx:      ctx,
		nodes:    nodes,
		entities: entities,
		facts:    facts,
		rels:     rels,
		svc:      NewService(entities, facts, rels, manuscript, nodes),
		pid:      p.ID,
		sceneID:  *p.LastOpenedNodeID,
	}
}

func TestResolveTargetFromEntity(t *testing.T) {
	f := newContextualFixture(t)
	e, err := f.entities.Create(f.ctx, 1_100, entity.NewInput{ProjectID: f.pid, Kind: entity.KindCharacter, Name: "민호", Role: "주인공"})
	if err != nil {
		t.Fatalf("Create entity: %v", err)
	}

	target, err := f.svc.ResolveTarget(f.ctx, ResolveTargetInput{ProjectID: f.pid, EntityID: e.ID})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target.CanonicalName != "민호" || target.Kind != entity.KindCharacter || len(target.EntityIDs) != 1 {
		t.Fatalf("target = %+v", target)
	}
}

func TestPlanAndApplyContextChangeRenamesEntityAndManuscript(t *testing.T) {
	f := newContextualFixture(t)
	e, err := f.entities.Create(f.ctx, 1_100, entity.NewInput{ProjectID: f.pid, Kind: entity.KindCharacter, Name: "민호", Role: "주인공"})
	if err != nil {
		t.Fatalf("Create entity: %v", err)
	}
	if err := f.nodes.UpdateContent(f.ctx, f.sceneID, contextDoc("민호는 고지서를 보았다. 민호는 한숨을 쉬었다."), 1_200); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	plan, err := f.svc.PlanContextChange(f.ctx, ChangeInput{
		ProjectID: f.pid,
		EntityID:  e.ID,
		Type:      ChangeTypeRename,
		NewTerms:  []string{"민준"},
	})
	if err != nil {
		t.Fatalf("PlanContextChange: %v", err)
	}
	if len(plan.MetadataCandidates) != 1 || plan.MetadataCandidates[0].Before != "민호" || plan.MetadataCandidates[0].After != "민준" {
		t.Fatalf("metadata candidates = %+v", plan.MetadataCandidates)
	}
	if len(plan.ManuscriptPlans) != 1 || len(plan.ManuscriptPlans[0].Candidates) != 1 {
		t.Fatalf("manuscript plans = %+v", plan.ManuscriptPlans)
	}

	applied, err := f.svc.ApplyContextChange(f.ctx, plan, ApplySelection{
		MetadataCandidateIDs: []string{plan.MetadataCandidates[0].ID},
		ManuscriptCandidateIDs: map[string][]string{
			plan.ManuscriptPlans[0].ID: []string{plan.ManuscriptPlans[0].Candidates[0].ID},
		},
	}, 2_000)
	if err != nil {
		t.Fatalf("ApplyContextChange: %v", err)
	}
	if applied.MetadataApplied != 1 || applied.Manuscript.Applied != 1 {
		t.Fatalf("applied = %+v", applied)
	}
	renamed, _ := f.entities.Get(f.ctx, e.ID)
	if renamed.Name != "민준" || renamed.Role != "주인공" {
		t.Fatalf("entity after apply = %+v", renamed)
	}
	scene, _ := f.nodes.Get(f.ctx, f.sceneID)
	if scene.ContentDoc == nil || strings.Contains(*scene.ContentDoc, "민호") || !strings.Contains(*scene.ContentDoc, "민준") {
		t.Fatalf("scene after apply = %s", valueOrEmpty(scene.ContentDoc))
	}
}

func TestConsistencyReportFindsRemainingOldTermAndFactReview(t *testing.T) {
	f := newContextualFixture(t)
	e, err := f.entities.Create(f.ctx, 1_100, entity.NewInput{ProjectID: f.pid, Kind: entity.KindCharacter, Name: "민호", Role: "주인공"})
	if err != nil {
		t.Fatalf("Create entity: %v", err)
	}
	if err := f.nodes.UpdateContent(f.ctx, f.sceneID, contextDoc("민호는 아직 옛 이름으로 불렸다."), 1_200); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if _, err := f.facts.Create(f.ctx, 1_300, fact.NewInput{
		ProjectID: f.pid,
		Claim:     "민호는 고지서를 싫어한다.",
		Result:    "민호 관련 설정",
		Sources:   []fact.SourceInput{{URL: "https://example.com/source"}},
	}); err != nil {
		t.Fatalf("Create fact: %v", err)
	}

	report, err := f.svc.CheckAfterChange(f.ctx, ConsistencyInput{
		ProjectID:        f.pid,
		OldTerms:         []string{"민호"},
		NewTerms:         []string{"민준"},
		ChangedEntityIDs: []string{e.ID},
	})
	if err != nil {
		t.Fatalf("CheckAfterChange: %v", err)
	}
	if report.OK || len(report.Issues) < 2 {
		t.Fatalf("report = %+v, want remaining manuscript and metadata/fact issues", report)
	}
}

func contextDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
