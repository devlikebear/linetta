package summarizer

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
)

type fixedProvider string

func (p fixedProvider) Provider() string { return string(p) }

type fakeClient struct {
	mu       sync.Mutex
	calls    [][]llm.ChatMessage
	response string
	block    chan struct{}
}

func (f *fakeClient) Ask(ctx context.Context, prompt string) (string, error) {
	return "", errors.New("not used")
}

func (f *fakeClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return llm.ChatResponse{}, ctx.Err()
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, messages)
	f.mu.Unlock()
	return llm.ChatResponse{Message: llm.ChatMessage{Content: f.response}}, nil
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newFixture(t *testing.T) (*store.Store, *node.Repo, project.Project) {
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
	return s, node.NewRepo(s), p
}

func longDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", msg)
}

func TestSummarizer_writesSummaryAndMatchesVersion(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	body := ""
	for i := 0; i < 200; i++ {
		body += "가나다라마"
	}
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body), 1100)

	fake := &fakeClient{response: "이것은 요약문이다."}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary == "이것은 요약문이다."
	}, "summary lands")

	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	if n.SummaryForVersion != n.ContentVersion {
		t.Errorf("versions: summary_for=%d content=%d", n.SummaryForVersion, n.ContentVersion)
	}
}

func TestSummarizer_skipsWhenAlreadyFresh(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	body := ""
	for i := 0; i < 200; i++ {
		body += "가"
	}
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body), 1100)
	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	_ = nodes.SetSummary(ctx, n.ID, "이미 요약됨.", n.ContentVersion)

	fake := &fakeClient{response: "새로 만든 요약."}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	time.Sleep(100 * time.Millisecond)

	if fake.callCount() != 0 {
		t.Errorf("LLM called %d times despite fresh cache", fake.callCount())
	}
}

func TestSummarizer_shortContent_writesPlaintextWithoutLLM(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc("짧다."), 1100)

	fake := &fakeClient{response: "should not be used"}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary != ""
	}, "short summary lands")

	if fake.callCount() != 0 {
		t.Errorf("LLM called %d times for short content", fake.callCount())
	}
}

func TestSummarizer_reRunsAfterContentChange(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	body := ""
	for i := 0; i < 200; i++ {
		body += "가"
	}
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body), 1100)

	fake := &fakeClient{response: "v1 요약"}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary == "v1 요약"
	}, "v1")

	body2 := ""
	for i := 0; i < 200; i++ {
		body2 += "나"
	}
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body2), 1200)
	fake.mu.Lock()
	fake.response = "v2 요약"
	fake.mu.Unlock()
	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary == "v2 요약"
	}, "v2")
}

func TestSummarizer_enqueueIsNonBlocking(t *testing.T) {
	_, nodes, _ := newFixture(t)
	ctx := context.Background()
	fake := &fakeClient{response: "ok", block: make(chan struct{})}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer func() {
		close(fake.block)
		stop()
	}()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10_000; i++ {
			sum.Enqueue("any-id")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Enqueue blocked under flood")
	}
}

// Compile-time guard that ai.ProviderSource is satisfied.
var _ ai.ProviderSource = fixedProvider("")
