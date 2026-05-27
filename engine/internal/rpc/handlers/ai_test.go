package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
	"github.com/devlikebear/tars/pkg/llm"
)

type fixedProvider string

func (p fixedProvider) Provider() string { return string(p) }

type capNotif struct {
	mu sync.Mutex
	es []string
}

func (n *capNotif) Notify(method string, _ any) error {
	n.mu.Lock()
	n.es = append(n.es, method)
	n.mu.Unlock()
	return nil
}

type streamingFake struct{}

func (streamingFake) Ask(_ context.Context, _ string) (string, error) {
	return "", errors.New("unused")
}
func (streamingFake) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if opts.OnDelta != nil {
		opts.OnDelta("결과")
	}
	// Return a ChatResponse with a single assistant message.
	// ChatMessage.Content is a string field in the tars API.
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "결과",
		},
	}, nil
}

func newAIFixture(t *testing.T) (*ai.Runner, *ai.ContextBuilder, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	nodes := node.NewRepo(s)
	mr := mention.NewRepo(s)
	runs := store.NewAIRunsRepo(s)
	notif := &capNotif{}
	runner := ai.NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return streamingFake{}, nil }, fixedProvider("claude-code-cli"))
	builder := ai.NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	return runner, builder, p.ID, *p.LastOpenedNodeID
}

func TestRunAIHandler_returnsRunID(t *testing.T) {
	runner, builder, _, nID := newAIFixture(t)
	h := RunAI(builder, runner, func() int64 { return 1 })

	params := json.RawMessage(`{"node_id":"` + nID + `","prompt":"안녕","options":{}}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(res, &got)
	if got["run_id"] == "" || got["run_id"] == nil {
		t.Errorf("missing run_id: %+v", got)
	}
	time.Sleep(50 * time.Millisecond) // let the goroutine finish so t.Cleanup doesn't race
}

func TestCancelAIHandler_unknownRunID(t *testing.T) {
	runner, _, _, _ := newAIFixture(t)
	h := CancelAI(runner)
	if _, err := h(context.Background(), json.RawMessage(`{"run_id":"nope"}`)); err == nil {
		t.Error("expected error for unknown run_id")
	}
}
