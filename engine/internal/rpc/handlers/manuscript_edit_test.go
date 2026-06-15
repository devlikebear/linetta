package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newManuscriptEditService(t *testing.T) (*manuscriptedit.Service, *node.Repo, string, string) {
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
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"수아는 진홍빛 열쇠를 품에 넣었다."}]}]}`
	if err := nodes.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 1_100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	return manuscriptedit.NewService(nodes, snapshot.NewRepo(st)), nodes, p.ID, *p.LastOpenedNodeID
}

func TestReplacePreviewHandlerReturnsCandidates(t *testing.T) {
	svc, _, projectID, nodeID := newManuscriptEditService(t)
	params, _ := json.Marshal(map[string]any{
		"project_id":  projectID,
		"query":       "진홍빛",
		"replacement": "푸른",
	})
	res, err := ReplacePreview(svc)(context.Background(), params)
	if err != nil {
		t.Fatalf("ReplacePreview: %v", err)
	}
	var plan manuscriptedit.ReplacePlan
	if err := json.Unmarshal(res, &plan); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].NodeID != nodeID {
		t.Fatalf("candidates = %+v, want node %s", plan.Candidates, nodeID)
	}
	if plan.Candidates[0].PreviewVersion == 0 || plan.Candidates[0].ID == "" {
		t.Fatalf("candidate should include id and preview version: %+v", plan.Candidates[0])
	}
}

func TestReplaceApplyHandlerAppliesSelectedCandidate(t *testing.T) {
	svc, nodes, projectID, nodeID := newManuscriptEditService(t)
	plan, err := svc.PlanReplace(context.Background(), manuscriptedit.ReplacePlanRequest{
		ProjectID: projectID, Query: "진홍빛", Replacement: "푸른",
	})
	if err != nil {
		t.Fatalf("PlanReplace: %v", err)
	}
	params, _ := json.Marshal(map[string]any{
		"plan":          plan,
		"candidate_ids": []string{plan.Candidates[0].ID},
	})
	res, err := ReplaceApply(svc, func() int64 { return 2_000 })(context.Background(), params)
	if err != nil {
		t.Fatalf("ReplaceApply: %v", err)
	}
	var result manuscriptedit.ApplyReplaceResult
	if err := json.Unmarshal(res, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Applied != 1 || len(result.ChangedNodeIDs) != 1 || result.ChangedNodeIDs[0] != nodeID {
		t.Fatalf("result = %+v, want changed node %s", result, nodeID)
	}
	got, _ := nodes.Get(context.Background(), nodeID)
	if got.ContentDoc == nil || !json.Valid([]byte(*got.ContentDoc)) {
		t.Fatalf("updated doc should be valid json: %v", got.ContentDoc)
	}
}

func TestReplaceHandlersRejectInvalidParams(t *testing.T) {
	svc, _, projectID, _ := newManuscriptEditService(t)
	for _, tc := range []struct {
		name    string
		handler rpc.Handler
		params  json.RawMessage
	}{
		{"preview missing", ReplacePreview(svc), json.RawMessage(`{}`)},
		{"preview empty query", ReplacePreview(svc), json.RawMessage(`{"project_id":"` + projectID + `","query":" ","replacement":"x"}`)},
		{"preview empty replacement", ReplacePreview(svc), json.RawMessage(`{"project_id":"` + projectID + `","query":"x","replacement":" "}`)},
		{"apply missing plan", ReplaceApply(svc, func() int64 { return 1 }), json.RawMessage(`{}`)},
	} {
		_, err := tc.handler(context.Background(), tc.params)
		var me *rpc.MethodError
		if !errors.As(err, &me) || me.Code != rpc.CodeInvalidParams {
			t.Fatalf("%s: expected invalid params, got %T %v", tc.name, err, err)
		}
	}
}
