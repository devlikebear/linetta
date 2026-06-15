package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/contextualedit"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type contextualHandlerFixture struct {
	svc       *contextualedit.Service
	nodes     *node.Repo
	entities  *entity.Repo
	projectID string
	sceneID   string
	entityID  string
}

func newContextualHandlerFixture(t *testing.T) contextualHandlerFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 1_000, project.NewInput{
		Title: "도시의 밤", Genres: []string{"mystery"}, LengthTarget: project.LengthShort, DefaultPOV: project.POVFirst,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	nodes := node.NewRepo(st)
	sceneID := *p.LastOpenedNodeID
	if err := nodes.UpdateContent(ctx, sceneID, contextualHandlerDoc("민호는 오래된 열쇠를 들었다."), 1_100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	entities := entity.NewRepo(st)
	e, err := entities.Create(ctx, 1_200, entity.NewInput{ProjectID: p.ID, Kind: entity.KindCharacter, Name: "민호", Role: "주인공"})
	if err != nil {
		t.Fatalf("Create entity: %v", err)
	}
	facts := fact.NewRepo(st)
	rels := relationship.NewRepo(st)
	manuscript := manuscriptedit.NewService(nodes, snapshot.NewRepo(st))
	return contextualHandlerFixture{
		svc:       contextualedit.NewService(entities, facts, rels, manuscript, nodes),
		nodes:     nodes,
		entities:  entities,
		projectID: p.ID,
		sceneID:   sceneID,
		entityID:  e.ID,
	}
}

func TestContextualPlanAndApplyHandlers(t *testing.T) {
	f := newContextualHandlerFixture(t)
	ctx := context.Background()
	params, _ := json.Marshal(map[string]any{
		"project_id": f.projectID,
		"entity_id":  f.entityID,
		"type":       contextualedit.ChangeTypeRename,
		"new_terms":  []string{"민준"},
	})
	rawPlan, err := ContextualPlanChange(f.svc)(ctx, params)
	if err != nil {
		t.Fatalf("ContextualPlanChange: %v", err)
	}
	var plan contextualedit.ChangePlan
	if err := json.Unmarshal(rawPlan, &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}
	if len(plan.MetadataCandidates) != 1 || len(plan.ManuscriptPlans) != 1 {
		t.Fatalf("plan = %+v", plan)
	}

	applyParams, _ := json.Marshal(map[string]any{
		"plan": plan,
		"selection": contextualedit.ApplySelection{
			MetadataCandidateIDs: []string{plan.MetadataCandidates[0].ID},
			ManuscriptCandidateIDs: map[string][]string{
				plan.ManuscriptPlans[0].ID: []string{plan.ManuscriptPlans[0].Candidates[0].ID},
			},
		},
	})
	rawResult, err := ContextualApplyChange(f.svc, func() int64 { return 2_000 })(ctx, applyParams)
	if err != nil {
		t.Fatalf("ContextualApplyChange: %v", err)
	}
	var result contextualedit.ApplyResult
	if err := json.Unmarshal(rawResult, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.MetadataApplied != 1 || result.Manuscript.Applied != 1 {
		t.Fatalf("result = %+v", result)
	}
	updated, _ := f.entities.Get(ctx, f.entityID)
	if updated.Name != "민준" {
		t.Fatalf("entity name = %q, want 민준", updated.Name)
	}
}

func TestContextualCheckConsistencyHandler(t *testing.T) {
	f := newContextualHandlerFixture(t)
	params, _ := json.Marshal(contextualedit.ConsistencyInput{ProjectID: f.projectID, OldTerms: []string{"민호"}})
	raw, err := ContextualCheckConsistency(f.svc)(context.Background(), params)
	if err != nil {
		t.Fatalf("ContextualCheckConsistency: %v", err)
	}
	var report contextualedit.ConsistencyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report.OK || len(report.Issues) == 0 {
		t.Fatalf("report = %+v, want issue", report)
	}
}

func TestContextualHandlersRejectInvalidParams(t *testing.T) {
	f := newContextualHandlerFixture(t)
	for _, tc := range []struct {
		name    string
		handler rpc.Handler
		params  json.RawMessage
	}{
		{"resolve missing project", ContextualResolveTarget(f.svc), json.RawMessage(`{}`)},
		{"plan missing project", ContextualPlanChange(f.svc), json.RawMessage(`{}`)},
		{"apply missing plan", ContextualApplyChange(f.svc, func() int64 { return 1 }), json.RawMessage(`{}`)},
		{"check missing terms", ContextualCheckConsistency(f.svc), json.RawMessage(`{"project_id":"` + f.projectID + `"}`)},
	} {
		_, err := tc.handler(context.Background(), tc.params)
		var me *rpc.MethodError
		if !errors.As(err, &me) || me.Code != rpc.CodeInvalidParams {
			t.Fatalf("%s: expected invalid params, got %T %v", tc.name, err, err)
		}
	}
}

func contextualHandlerDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}
