package summarizer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
	mu         sync.Mutex
	calls      [][]llm.ChatMessage
	response   string
	responseFn func(messages []llm.ChatMessage) string
	block      chan struct{}
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
	fn := f.responseFn
	resp := f.response
	f.mu.Unlock()
	if fn != nil {
		return llm.ChatResponse{Message: llm.ChatMessage{Content: fn(messages)}}, nil
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Content: resp}}, nil
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

func TestSummarizer_containerRollupBuildsDepth2Tree(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()

	// 부 → 장 → 씬 tree (with 2 scenes).
	part, _ := nodes.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", 1100)
	chap, _ := nodes.CreateChild(ctx, part.ID, "container", "1장", "", 1110)
	scene1, _ := nodes.CreateChild(ctx, chap.ID, "leaf", "씬 1", "", 1120)
	scene2, _ := nodes.CreateChild(ctx, chap.ID, "leaf", "씬 2", "", 1130)

	body := ""
	for i := 0; i < 200; i++ {
		body += "가나다라마"
	}
	_ = nodes.UpdateContent(ctx, scene1.ID, longDoc(body), 1200)
	_ = nodes.UpdateContent(ctx, scene2.ID, longDoc(body), 1210)

	type capture struct {
		input string
		reply string
	}
	var (
		mu    sync.Mutex
		calls []capture
	)
	fake := &fakeClient{}
	fake.responseFn = func(messages []llm.ChatMessage) string {
		mu.Lock()
		defer mu.Unlock()
		userMsg := messages[len(messages)-1].Content
		reply := fmt.Sprintf("요약#%d", len(calls)+1)
		calls = append(calls, capture{input: userMsg, reply: reply})
		return reply
	}

	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(part.ID)

	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, part.ID)
		return n.Summary != "" && n.SummaryForVersion == n.ContentVersion
	}, "part summary lands")

	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 4 {
		// 4 = 씬 1, 씬 2, 1장, 1부.
		t.Fatalf("LLM call count = %d, want >= 4; calls=%+v", len(calls), calls)
	}
	// The 1장 call's input must include both 씬 1 and 씬 2 labels.
	var chapInput string
	for _, c := range calls {
		if strings.Contains(c.input, "씬 1") && strings.Contains(c.input, "씬 2") {
			chapInput = c.input
			break
		}
	}
	if chapInput == "" {
		t.Errorf("no 1장 rollup call found; calls = %+v", calls)
	}

	// Both 장 and 부 should have stored summaries with matching content_version.
	gotChap, _ := nodes.Get(ctx, chap.ID)
	if gotChap.Summary == "" || gotChap.SummaryForVersion != gotChap.ContentVersion {
		t.Errorf("chap.summary not fresh: %+v", gotChap)
	}
	gotPart, _ := nodes.Get(ctx, part.ID)
	if gotPart.Summary == "" || gotPart.SummaryForVersion != gotPart.ContentVersion {
		t.Errorf("part.summary not fresh: %+v", gotPart)
	}
}

func TestSummarizer_containerDepthCap_stopsBeyondDepth6(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()

	// Build a chain of 7 containers under the seeded root leaf's project.
	// First container is a sibling of the seeded leaf; subsequent ones nest as
	// children. Total depth from root container down: 7 containers + 1 leaf.
	var chain []node.Node
	parent, _ := nodes.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "C0", "", 1100)
	chain = append(chain, parent)
	for i := 1; i < 7; i++ {
		c, _ := nodes.CreateChild(ctx, parent.ID, "container", fmt.Sprintf("C%d", i), "", int64(1100+i))
		chain = append(chain, c)
		parent = c
	}
	leaf, _ := nodes.CreateChild(ctx, parent.ID, "leaf", "씬", "", 1200)

	body := ""
	for i := 0; i < 200; i++ {
		body += "가나다라마"
	}
	_ = nodes.UpdateContent(ctx, leaf.ID, longDoc(body), 1300)

	var mu sync.Mutex
	callsAt := map[string]int{}
	fake := &fakeClient{}
	fake.responseFn = func(messages []llm.ChatMessage) string {
		mu.Lock()
		defer mu.Unlock()
		// Tag the system prompt so we can distinguish leaf vs container calls if needed.
		callsAt[messages[0].Content]++
		return "rolled-up"
	}

	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(chain[0].ID) // depth 0 → child depth 1 ... → leaf would be depth 7 (capped).

	// Wait a bit; cap means the leaf at depth 7 is NOT summarized.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		stable := len(callsAt) >= 1
		mu.Unlock()
		if stable {
			time.Sleep(100 * time.Millisecond)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	gotLeaf, _ := nodes.Get(ctx, leaf.ID)
	if gotLeaf.Summary != "" {
		t.Errorf("depth-cap breached: deepest leaf got summarized: %q", gotLeaf.Summary)
	}
}

func TestSummarizer_container_skipsLLMWhenChildrenFresh(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()

	part, _ := nodes.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", 1100)
	chap, _ := nodes.CreateChild(ctx, part.ID, "container", "1장", "", 1110)
	leaf, _ := nodes.CreateChild(ctx, chap.ID, "leaf", "씬 1", "", 1120)

	body := ""
	for i := 0; i < 200; i++ {
		body += "가나다라마"
	}
	_ = nodes.UpdateContent(ctx, leaf.ID, longDoc(body), 1200)

	// Pre-seed fresh summaries on the leaf and intermediate chap so the only
	// stale node is `part`.
	leafGot, _ := nodes.Get(ctx, leaf.ID)
	_ = nodes.SetSummary(ctx, leaf.ID, "씬1 요약", leafGot.ContentVersion)
	chapGot, _ := nodes.Get(ctx, chap.ID)
	_ = nodes.SetSummary(ctx, chap.ID, "1장 요약", chapGot.ContentVersion)

	fake := &fakeClient{response: "최종 부 요약"}

	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(part.ID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, part.ID)
		return n.Summary == "최종 부 요약"
	}, "part summary lands")

	if got := fake.callCount(); got != 1 {
		t.Errorf("LLM call count = %d, want 1 (only the part-level rollup)", got)
	}
}

// Compile-time guard that ai.ProviderSource is satisfied.
var _ ai.ProviderSource = fixedProvider("")
