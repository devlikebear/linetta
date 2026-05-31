package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/search"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newSearchRepo(t *testing.T) (*search.Repo, string) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	projects := project.NewRepo(s)
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
	if err := node.NewRepo(s).UpdateContent(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"숨은 열쇠를 발견했다."}]}]}`, 1_100); err != nil {
		t.Fatalf("update content: %v", err)
	}
	return search.NewRepo(s), nodeID
}

func TestSearchHandlerReturnsResults(t *testing.T) {
	repo, nodeID := newSearchRepo(t)
	res, err := Search(repo)(context.Background(), json.RawMessage(`{"query":"열쇠","limit":5}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out []search.Result
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 || out[0].NodeID != nodeID {
		t.Fatalf("results = %#v, want node %q", out, nodeID)
	}
}

func TestSearchHandlerRejectsBlankQuery(t *testing.T) {
	repo, _ := newSearchRepo(t)
	_, err := Search(repo)(context.Background(), json.RawMessage(`{"query":"   "}`))
	me, ok := err.(*rpc.MethodError)
	if !ok || me.Code != rpc.CodeInvalidParams {
		t.Fatalf("err = %v, want invalid params", err)
	}
}
