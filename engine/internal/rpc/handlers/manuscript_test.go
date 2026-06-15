package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newManuscriptSearcher(t *testing.T) (*manuscript.Searcher, string, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	indexer := manuscript.NewIndexer(st.DB())
	nodes.SetManuscriptIndexer(indexer)
	searcher := manuscript.NewSearcher(st.DB(), nodes, indexer)
	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title:        "도시의 밤",
		Genres:       []string{"mystery"},
		LengthTarget: project.LengthShort,
		DefaultPOV:   project.POVFirst,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	nodeID := *p.LastOpenedNodeID
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"수아는 진홍빛 열쇠를 품에 넣었다."}]}]}`
	if err := nodes.UpdateContent(ctx, nodeID, doc, 1_100); err != nil {
		t.Fatalf("update content: %v", err)
	}
	return searcher, p.ID, nodeID
}

func TestSearchManuscriptHandlerReturnsHits(t *testing.T) {
	searcher, projectID, nodeID := newManuscriptSearcher(t)
	params, _ := json.Marshal(map[string]any{
		"project_id": projectID,
		"query":      "진홍빛",
		"limit":      5,
	})
	res, err := SearchManuscript(searcher)(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out []manuscript.Hit
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 || out[0].NodeID != nodeID {
		t.Fatalf("hits = %#v, want node %q", out, nodeID)
	}
	if out[0].Breadcrumb == "" || out[0].Snippet == "" || out[0].UpdatedAt == 0 {
		t.Fatalf("hit should include breadcrumb, snippet, updated_at: %#v", out[0])
	}
}

func TestSearchManuscriptHandlerRejectsInvalidParams(t *testing.T) {
	searcher, projectID, _ := newManuscriptSearcher(t)
	for _, params := range []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"project_id":"p1"}`),
		json.RawMessage(`{"query":"열쇠"}`),
		json.RawMessage(`{"project_id":"` + projectID + `","query":"   "}`),
	} {
		_, err := SearchManuscript(searcher)(context.Background(), params)
		var me *rpc.MethodError
		if !errors.As(err, &me) || me.Code != rpc.CodeInvalidParams {
			t.Fatalf("params %s: expected invalid params, got %T %v", params, err, err)
		}
	}
}
