package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	"github.com/devlikebear/tars/pkg/session"
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

func (p fixedProvider) Resolve() ai.ResolvedProvider {
	return ai.ResolvedProvider{Provider: string(p)}
}

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
	fc := &fakeClient{responses: responses}
	return newSvcWithClient(t, fc)
}

func newSvcWithClient(t *testing.T, client llm.Client) (*Service, *fakeNotifier, string) {
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
	svc := NewService(t.TempDir(), projects, threads, entities, rels, pb, notif,
		func(ai.ResolvedProvider) (llm.Client, error) { return client, nil }, fixedProvider("claude-code-cli"), "",
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

func TestSend_RetriesDirectMutationWhenModelOnlyClaimsApplied(t *testing.T) {
	client := &claimThenApplyClient{}
	svc, notif, projectID := newSvcWithClient(t, client)
	runID, err := svc.Send(context.Background(), projectID, "", "아웃라인을 새로 작성해줘", func() int64 { return 1000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	waitFor(t, notif, "companion.done")

	client.mu.Lock()
	calls := client.calls
	firstChoice := client.firstChoice
	firstTools := append([]string(nil), client.firstTools...)
	sawCorrection := client.sawCorrection
	sawToolResult := client.sawToolResult
	client.mu.Unlock()

	if calls < 3 {
		t.Fatalf("expected retry, tool call, and final response, got %d calls", calls)
	}
	if firstChoice == nil || firstChoice.Mode != llm.ToolChoiceModeRequired {
		t.Fatalf("first tool choice = %+v, want required", firstChoice)
	}
	if len(firstTools) != 1 || firstTools[0] != applyOpsToolName {
		t.Fatalf("first tools = %+v, want only %s", firstTools, applyOpsToolName)
	}
	if !sawCorrection {
		t.Fatal("expected corrective prompt after a direct mutation without tool use")
	}
	if !sawToolResult {
		t.Fatal("expected final turn to receive apply-ops tool result")
	}
	if got := notif.get("companion.applied"); !strings.Contains(got, `"applied":2`) {
		t.Fatalf("expected applied event, got %s", got)
	}
	nodes, err := svc.nodes.ListByProject(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	foundPart := false
	foundScene := false
	for _, n := range nodes {
		if n.Label == "1부" && n.Title == "항구의 복수극" {
			foundPart = true
		}
		if n.Label == "씬 1" && n.Title == "안개 낀 항구" && n.ParentID != nil {
			foundScene = true
		}
	}
	if !foundPart || !foundScene {
		t.Fatalf("outline tree was not applied: %+v", nodes)
	}
}

type claimThenApplyClient struct {
	mu            sync.Mutex
	calls         int
	firstChoice   *llm.ToolChoice
	firstTools    []string
	sawCorrection bool
	sawToolResult bool
}

func (c *claimThenApplyClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *claimThenApplyClient) Chat(_ context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	switch c.calls {
	case 1:
		c.firstChoice = opts.ToolChoice
		c.firstTools = toolSchemaNames(opts.Tools)
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "아웃라인을 반영했습니다.",
		}}, nil
	case 2:
		if len(messages) > 0 && strings.Contains(messages[len(messages)-1].Content, "실제 작품 상태 변경") {
			c.sawCorrection = true
		}
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:        "call_apply",
				Name:      applyOpsToolName,
				Arguments: `{"summary":"아웃라인 작성","ops_json":"[{\"op\":\"create_outline_node\",\"ref\":\"p1\",\"kind\":\"container\",\"label\":\"1부\",\"title\":\"항구의 복수극\"},{\"op\":\"create_outline_node\",\"ref\":\"s1\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 1\",\"title\":\"안개 낀 항구\"}]"}`,
			}},
		}}, nil
	default:
		if len(messages) > 0 && messages[len(messages)-1].Role == "tool" && strings.Contains(messages[len(messages)-1].Content, `"applied":2`) {
			c.sawToolResult = true
		}
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "아웃라인을 실제로 반영했습니다.",
		}}, nil
	}
}

func TestCancel_UnknownRunErrors(t *testing.T) {
	svc, _, _ := newSvc(t, "안녕")
	if err := svc.Cancel("no-such-run"); err == nil {
		t.Fatal("expected error cancelling unknown run")
	}
}

func TestCompactHistory_RewritesTranscriptAsSummary(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	sess, err := svc.sessions.EnsureWorker(projectID)
	if err != nil {
		t.Fatal(err)
	}
	path := svc.sessions.TranscriptPath(sess.ID)
	at := time.UnixMilli(1000)
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: "첫 장면을 더 불안하게 만들고 싶어", Timestamp: at}); err != nil {
		t.Fatal(err)
	}
	if err := session.AppendMessage(path, session.Message{Role: "assistant", Content: "비 오는 창문과 문자를 늦게 보여주면 긴장이 생깁니다.", Timestamp: at}); err != nil {
		t.Fatal(err)
	}

	msgs, err := svc.CompactHistory(context.Background(), projectID, func() int64 { return 2000 })
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Role != "assistant" {
		t.Fatalf("compact messages = %+v", msgs)
	}
	if !strings.Contains(msgs[0].Content, "이전 컴패니언 대화 요약") ||
		!strings.Contains(msgs[0].Content, "나: 첫 장면을 더 불안하게") ||
		!strings.Contains(msgs[0].Content, "컴패니언: 비 오는 창문") {
		t.Fatalf("summary missing transcript details: %q", msgs[0].Content)
	}
	history, err := svc.History(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Content != msgs[0].Content {
		t.Fatalf("history not compacted: %+v", history)
	}
}

func TestClearHistory_RemovesTranscriptMessages(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	sess, err := svc.sessions.EnsureWorker(projectID)
	if err != nil {
		t.Fatal(err)
	}
	path := svc.sessions.TranscriptPath(sess.ID)
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: "지울 대화", Timestamp: time.UnixMilli(1000)}); err != nil {
		t.Fatal(err)
	}

	if err := svc.ClearHistory(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}
	msgs, err := svc.History(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected empty history, got %+v", msgs)
	}
}

func TestDeleteProjectData_RemovesTranscriptAndMemory(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	sess, err := svc.sessions.EnsureWorker(projectID)
	if err != nil {
		t.Fatal(err)
	}
	path := svc.sessions.TranscriptPath(sess.ID)
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: "삭제될 대화", Timestamp: time.UnixMilli(1000)}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remember(projectID, "삭제될 기억", "fact"); err != nil {
		t.Fatal(err)
	}
	memPath := memRoot(svc.memBase, projectID)

	if err := svc.DeleteProjectData(context.Background(), projectID); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("transcript stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(memPath); !os.IsNotExist(err) {
		t.Fatalf("memory stat err = %v, want not exist", err)
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

func TestGatherContext_PrioritizesCoreEntitiesPastRecentLimit(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	ctx := context.Background()
	core, err := svc.entities.Create(ctx, 10, entity.NewInput{
		ProjectID: projectID, Kind: "place", Name: "망각의 항구", Role: "특별한 장소",
	})
	if err != nil {
		t.Fatalf("create core entity: %v", err)
	}
	for i := 0; i < entityContextLimit+5; i++ {
		_, err := svc.entities.Create(ctx, int64(100+i), entity.NewInput{
			ProjectID: projectID,
			Kind:      "character",
			Name:      "최근 인물 " + fmt.Sprint(i),
			Role:      "단역",
		})
		if err != nil {
			t.Fatalf("create filler %d: %v", i, err)
		}
	}

	d, err := svc.gatherContext(ctx, projectID, "", "항구 설정")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range d.Entities {
		if e.ID == core.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("core entity %q missing from companion context of %d entities", core.Name, len(d.Entities))
	}
}

func TestSend_surfacesTranscriptPersistenceError(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	sess, err := svc.sessions.EnsureWorker(projectID)
	if err != nil {
		t.Fatal(err)
	}
	path := svc.sessions.TranscriptPath(sess.ID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	runID, err := svc.Send(context.Background(), projectID, "", "기록돼야 할 메시지", func() int64 { return 1 })
	if err == nil {
		t.Fatalf("expected transcript persistence error, runID=%q", runID)
	}
	if !strings.Contains(err.Error(), "transcript") {
		t.Fatalf("expected transcript error, got %v", err)
	}
}
