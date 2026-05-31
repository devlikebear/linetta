package companion

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

type fakeClient struct {
	mu        sync.Mutex
	responses []string
	idx       int
}

func (f *fakeClient) Ask(context.Context, string) (string, error) { return "", nil }
func (f *fakeClient) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	f.mu.Lock()
	resp := ""
	if len(f.responses) > 0 {
		if f.idx >= len(f.responses) {
			resp = f.responses[len(f.responses)-1]
		} else {
			resp = f.responses[f.idx]
		}
		f.idx++
	}
	f.mu.Unlock()
	if opts.OnDelta != nil {
		opts.OnDelta(resp)
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: resp}}, nil
}

type fakeNotifier struct {
	mu     sync.Mutex
	events map[string]string
}

func (n *fakeNotifier) Notify(method string, params any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.events == nil {
		n.events = map[string]string{}
	}
	b, _ := json.Marshal(params)
	n.events[method] = string(b)
	return nil
}
func (n *fakeNotifier) get(method string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.events[method]
}

type fixedProvider string

func (p fixedProvider) Provider() string { return string(p) }

func waitFor(t *testing.T, n *fakeNotifier, method string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if n.get(method) != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s notification", method)
}

func newSvc(t *testing.T, full string) (*Service, *fakeNotifier, string) {
	t.Helper()
	return newSvcQueue(t, []string{full})
}

func newSvcQueue(t *testing.T, responses []string) (*Service, *fakeNotifier, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	entities := entity.NewRepo(st)
	rels := relationship.NewRepo(st)
	pb := plot.NewBuilder(nodes, beats, threads)
	notif := &fakeNotifier{}
	fc := &fakeClient{responses: responses}
	svc := NewService(t.TempDir(), projects, threads, entities, rels, pb, notif,
		func(_, _ string) (llm.Client, error) { return fc, nil }, fixedProvider("claude-code-cli"), "",
		nodes, beats)
	p, err := projects.Create(context.Background(), 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, notif, p.ID
}

func TestSend_StreamsDoneProposalAndPersists(t *testing.T) {
	full := "좋아요! 제안할게요.\n```linetta-proposal\n{\"summary\":\"복수극\",\"ops\":[{\"op\":\"set_outline\",\"outline\":\"복수 서사\"}]}\n```"
	svc, notif, projectID := newSvc(t, full)
	runID, err := svc.Send(context.Background(), projectID, "", "복수극 어때?", func() int64 { return 1000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	waitFor(t, notif, "companion.done")
	if !strings.Contains(notif.get("companion.done"), "복수 서사") {
		t.Fatalf("done missing full text: %s", notif.get("companion.done"))
	}
	if !strings.Contains(notif.get("companion.proposal"), "\"valid\":true") {
		t.Fatalf("expected valid proposal: %s", notif.get("companion.proposal"))
	}
	msgs, err := svc.History(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("transcript = %+v", msgs)
	}
}

func TestSend_NoProposalWhenNoBlock(t *testing.T) {
	svc, notif, projectID := newSvc(t, "그냥 수다입니다. 제안 없음.")
	if _, err := svc.Send(context.Background(), projectID, "", "안녕", func() int64 { return 1 }); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notif, "companion.done")
	if notif.get("companion.proposal") != "" {
		t.Fatalf("did not expect a proposal event: %s", notif.get("companion.proposal"))
	}
}

func TestRun_QueryThenFinal(t *testing.T) {
	// round0: a linetta-query; round1: final answer with a proposal.
	round0 := "찾아볼게요\n```linetta-query\n{\"queries\":[{\"tool\":\"list_scenes\",\"args\":{}}]}\n```"
	round1 := "이렇게 제안해요\n```linetta-proposal\n{\"summary\":\"s\",\"ops\":[{\"op\":\"set_outline\",\"outline\":\"x\"}]}\n```"
	svc, notif, projectID := newSvcQueue(t, []string{round0, round1})
	runID, err := svc.Send(context.Background(), projectID, "", "플롯 구상", func() int64 { return 1 })
	if err != nil || runID == "" {
		t.Fatal(err)
	}
	waitFor(t, notif, "companion.done")
	if notif.get("companion.thinking") == "" {
		t.Fatal("expected a thinking event during the query round")
	}
	if !strings.Contains(notif.get("companion.done"), "이렇게 제안해요") {
		t.Fatalf("final answer missing: %s", notif.get("companion.done"))
	}
	if !strings.Contains(notif.get("companion.proposal"), "\"valid\":true") {
		t.Fatalf("final proposal missing: %s", notif.get("companion.proposal"))
	}
	// transcript: user + only the FINAL assistant (query round not persisted)
	msgs, _ := svc.History(context.Background(), projectID)
	if len(msgs) != 2 {
		t.Fatalf("want 2 transcript msgs (user+final assistant), got %d: %+v", len(msgs), msgs)
	}
}

func TestCancel_UnknownRunErrors(t *testing.T) {
	svc, _, _ := newSvc(t, "안녕")
	if err := svc.Cancel("no-such-run"); err == nil {
		t.Fatal("expected error cancelling unknown run")
	}
}

func TestGatherContext_InjectsMemory(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	if err := svc.Remember(projectID, "작가는 반전을 좋아한다", "preference"); err != nil {
		t.Fatal(err)
	}
	d, err := svc.gatherContext(context.Background(), projectID, "", "반전")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range d.Memories {
		if m == "작가는 반전을 좋아한다" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory not recalled: %+v", d.Memories)
	}
}
