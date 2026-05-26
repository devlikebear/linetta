package ai

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
)

// fixedProvider is a tiny ProviderSource stub returning a constant id.
type fixedProvider string

func (p fixedProvider) Provider() string { return string(p) }

// fakeClient streams a fixed set of chunks. Implements llm.Client.
type fakeClient struct {
	chunks []string
	failAt int // index at which to return an error; -1 = never
	hold   chan struct{}
}

func (f *fakeClient) Ask(ctx context.Context, prompt string) (string, error) {
	return "", errors.New("not used")
}

func (f *fakeClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	var full strings.Builder
	for i, c := range f.chunks {
		if ctx.Err() != nil {
			return llm.ChatResponse{}, ctx.Err()
		}
		if f.hold != nil {
			select {
			case <-f.hold:
			case <-ctx.Done():
				return llm.ChatResponse{}, ctx.Err()
			}
		}
		if f.failAt == i {
			return llm.ChatResponse{}, errors.New("simulated provider failure")
		}
		full.WriteString(c)
		if opts.OnDelta != nil {
			opts.OnDelta(c)
		}
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Content: full.String()}}, nil
}

type fakeNotifier struct {
	mu     sync.Mutex
	events []string
}

func (n *fakeNotifier) Notify(method string, params any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, _ := json.Marshal(params)
	n.events = append(n.events, method+":"+string(b))
	return nil
}

func newRunnerFixture(t *testing.T) (*store.Store, project.Project) {
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
	return s, p
}

func TestRunner_streams_thenEmitsDone(t *testing.T) {
	s, p := newRunnerFixture(t)
	fake := &fakeClient{chunks: []string{"안녕 ", "세계", "!"}, failAt: -1}
	notif := &fakeNotifier{}

	runs := store.NewAIRunsRepo(s)
	r := NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return fake, nil }, fixedProvider("claude-code-cli"))
	now := func() int64 { return 1234 }

	c := Context{ProjectID: p.ID, NodeID: *p.LastOpenedNodeID, SceneLabel: "씬 1", UserPrompt: "안녕"}
	runID, err := r.Start(context.Background(), c, now)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if runID == "" {
		t.Fatal("missing runID")
	}

	deadline := time.Now().Add(1 * time.Second)
	for {
		notif.mu.Lock()
		count := len(notif.events)
		notif.mu.Unlock()
		if count >= 4 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()
	if len(notif.events) != 4 {
		t.Fatalf("events = %v", notif.events)
	}
	for i, expected := range []string{"ai.delta", "ai.delta", "ai.delta", "ai.done"} {
		if !strings.HasPrefix(notif.events[i], expected) {
			t.Errorf("event[%d] = %q, want prefix %q", i, notif.events[i], expected)
		}
	}
	if !strings.Contains(notif.events[3], "안녕 세계!") {
		t.Errorf("full_text missing: %q", notif.events[3])
	}

	// Persisted as done.
	rows, _ := runs.ListRecent(context.Background(), p.ID, 5)
	if len(rows) != 1 || rows[0].Status != store.AIRunDone || rows[0].Output != "안녕 세계!" {
		t.Errorf("persisted = %+v", rows)
	}
}

func TestRunner_cancel_emitsCancelled_andPersistsCancelled(t *testing.T) {
	s, p := newRunnerFixture(t)
	fake := &fakeClient{chunks: []string{"한", "두"}, failAt: -1, hold: make(chan struct{})}
	notif := &fakeNotifier{}
	runs := store.NewAIRunsRepo(s)
	r := NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return fake, nil }, fixedProvider("claude-code-cli"))
	now := func() int64 { return 1234 }

	c := Context{ProjectID: p.ID, NodeID: *p.LastOpenedNodeID, SceneLabel: "씬 1", UserPrompt: "안녕"}
	runID, err := r.Start(context.Background(), c, now)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the goroutine a moment to enter Chat and block on f.hold.
	time.Sleep(20 * time.Millisecond)

	if err := r.Cancel(runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for {
		notif.mu.Lock()
		cancelled := false
		for _, e := range notif.events {
			if strings.HasPrefix(e, "ai.cancelled") {
				cancelled = true
			}
		}
		notif.mu.Unlock()
		if cancelled || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	rows, _ := runs.ListRecent(context.Background(), p.ID, 5)
	if len(rows) != 1 || rows[0].Status != store.AIRunCancelled {
		t.Errorf("persisted = %+v", rows)
	}
}

type mutableProvider struct {
	mu sync.Mutex
	v  string
}

func (m *mutableProvider) Provider() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.v
}

func (m *mutableProvider) set(v string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.v = v
}

type recordingFactory struct {
	mu           sync.Mutex
	lastProvider string
	delegate     ClientFactory
}

func (rf *recordingFactory) build(provider, workDir string) (llm.Client, error) {
	rf.mu.Lock()
	rf.lastProvider = provider
	rf.mu.Unlock()
	return rf.delegate(provider, workDir)
}

func (rf *recordingFactory) last() string {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.lastProvider
}

func TestRunner_readsProviderOnEachStart(t *testing.T) {
	s, p := newRunnerFixture(t)
	notif := &fakeNotifier{}
	runs := store.NewAIRunsRepo(s)

	rf := &recordingFactory{delegate: func(_, _ string) (llm.Client, error) {
		return &fakeClient{chunks: []string{"x"}, failAt: -1}, nil
	}}
	src := &mutableProvider{v: "claude-code-cli"}
	r := NewRunner(notif, runs, rf.build, src)
	now := func() int64 { return 1 }

	c := Context{ProjectID: p.ID, NodeID: *p.LastOpenedNodeID, SceneLabel: "씬 1", UserPrompt: "안녕"}
	if _, err := r.Start(context.Background(), c, now); err != nil {
		t.Fatalf("first start: %v", err)
	}
	// Wait for first run to land.
	deadline := time.Now().Add(1 * time.Second)
	for {
		if got := rf.last(); got == "claude-code-cli" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("factory never observed first provider; got %q", rf.last())
		}
		time.Sleep(5 * time.Millisecond)
	}

	src.set("openai-codex")
	if _, err := r.Start(context.Background(), c, now); err != nil {
		t.Fatalf("second start: %v", err)
	}
	deadline = time.Now().Add(1 * time.Second)
	for {
		if got := rf.last(); got == "openai-codex" {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("factory called with %q on second start, want openai-codex", rf.last())
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunner_providerError_emitsError(t *testing.T) {
	s, p := newRunnerFixture(t)
	fake := &fakeClient{chunks: []string{"한", "두"}, failAt: 1} // error after first chunk
	notif := &fakeNotifier{}
	runs := store.NewAIRunsRepo(s)
	r := NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return fake, nil }, fixedProvider("claude-code-cli"))
	now := func() int64 { return 1234 }

	c := Context{ProjectID: p.ID, NodeID: *p.LastOpenedNodeID, SceneLabel: "씬 1", UserPrompt: "안녕"}
	if _, err := r.Start(context.Background(), c, now); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for {
		notif.mu.Lock()
		errored := false
		for _, e := range notif.events {
			if strings.HasPrefix(e, "ai.error") {
				errored = true
			}
		}
		notif.mu.Unlock()
		if errored || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}
