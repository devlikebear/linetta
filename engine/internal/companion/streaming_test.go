package companion

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
	"github.com/devlikebear/tars/pkg/llm"
)

// streamingClient emits several distinct deltas via OnDelta to mimic a real
// streaming provider (the existing fakeClient emits a single delta, which hid
// the streaming contract from tests).
type streamingClient struct {
	chunks    []string
	reasoning []string
}

func (c *streamingClient) Ask(context.Context, string) (string, error) { return "", nil }
func (c *streamingClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	for _, r := range c.reasoning {
		if opts.OnReasoningDelta != nil {
			opts.OnReasoningDelta(r)
		}
	}
	var full strings.Builder
	for _, ch := range c.chunks {
		full.WriteString(ch)
		if opts.OnDelta != nil {
			opts.OnDelta(ch)
		}
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: full.String()}}, nil
}

type orderedNotifier struct {
	mu  sync.Mutex
	seq []string
}

func (n *orderedNotifier) Notify(method string, _ any) error {
	n.mu.Lock()
	n.seq = append(n.seq, method)
	n.mu.Unlock()
	return nil
}

func (n *orderedNotifier) snapshot() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.seq...)
}

// The engine must emit companion.delta for each streamed chunk, all before
// companion.done — i.e. the panel receives incremental updates, not one dump
// at the end.
func TestCompanionStreamsDeltasIncrementally(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	entities := entity.NewRepo(st)
	rels := relationship.NewRepo(st)
	pb := plot.NewBuilder(nodes, beats, threads)
	notif := &orderedNotifier{}
	client := &streamingClient{chunks: []string{"안녕", "하세", "요 반갑", "습니다"}}
	svc := NewService(t.TempDir(), projects, threads, entities, rels, pb, notif,
		func(ai.ResolvedProvider) (llm.Client, error) { return client, nil }, fixedProvider("claude-code-cli"), "",
		nodes, beats)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	nodeID := ""
	if p.LastOpenedNodeID != nil {
		nodeID = *p.LastOpenedNodeID
	}

	if _, err := svc.Send(ctx, p.ID, nodeID, "안녕하세요", func() int64 { return 1 }); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		seq := notif.snapshot()
		done := false
		for _, m := range seq {
			if m == "companion.done" {
				done = true
			}
		}
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	seq := notif.snapshot()
	deltas, doneIdx := 0, -1
	for i, m := range seq {
		if m == "companion.delta" {
			deltas++
		}
		if m == "companion.done" && doneIdx < 0 {
			doneIdx = i
		}
	}
	if doneIdx < 0 {
		t.Fatalf("no companion.done emitted; seq=%v", seq)
	}
	if deltas < 2 {
		t.Fatalf("expected multiple companion.delta (incremental streaming), got %d; seq=%v", deltas, seq)
	}
	for i := doneIdx + 1; i < len(seq); i++ {
		if seq[i] == "companion.delta" {
			t.Fatalf("companion.delta emitted after companion.done; seq=%v", seq)
		}
	}
}

// The engine forwards provider reasoning deltas as companion.reasoning so the
// panel can show the AI's thinking.
func TestCompanionEmitsReasoning(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	entities := entity.NewRepo(st)
	rels := relationship.NewRepo(st)
	pb := plot.NewBuilder(nodes, beats, threads)
	notif := &orderedNotifier{}
	client := &streamingClient{reasoning: []string{"먼저 ", "구상한다"}, chunks: []string{"결과"}}
	svc := NewService(t.TempDir(), projects, threads, entities, rels, pb, notif,
		func(ai.ResolvedProvider) (llm.Client, error) { return client, nil }, fixedProvider("claude-code-cli"), "",
		nodes, beats)
	p, _ := projects.Create(ctx, 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	nodeID := ""
	if p.LastOpenedNodeID != nil {
		nodeID = *p.LastOpenedNodeID
	}

	if _, err := svc.Send(ctx, p.ID, nodeID, "안녕", func() int64 { return 1 }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		seq := notif.snapshot()
		done := false
		for _, m := range seq {
			if m == "companion.done" {
				done = true
			}
		}
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	seq := notif.snapshot()
	reasoning := 0
	for _, m := range seq {
		if m == "companion.reasoning" {
			reasoning++
		}
	}
	if reasoning < 2 {
		t.Fatalf("expected >=2 companion.reasoning, got %d in %v", reasoning, seq)
	}
}
