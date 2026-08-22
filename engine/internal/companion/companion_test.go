package companion

import (
	"context"
	"encoding/base64"
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
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
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

type captureMessagesClient struct {
	mu       sync.Mutex
	messages [][]llm.ChatMessage
}

func (c *captureMessagesClient) Ask(context.Context, string) (string, error) { return "", nil }
func (c *captureMessagesClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	copied := append([]llm.ChatMessage(nil), messages...)
	c.messages = append(c.messages, copied)
	c.mu.Unlock()
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "확인했습니다."}}, nil
}

type captureToolsClient struct {
	mu       sync.Mutex
	reply    string
	tools    []llm.ToolSchema
	messages []llm.ChatMessage
}

func (c *captureToolsClient) Ask(context.Context, string) (string, error) { return "", nil }
func (c *captureToolsClient) Chat(_ context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	c.tools = append([]llm.ToolSchema(nil), opts.Tools...)
	c.messages = append([]llm.ChatMessage(nil), messages...)
	c.mu.Unlock()
	if opts.OnDelta != nil {
		opts.OnDelta(c.reply)
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: c.reply}}, nil
}

func (c *captureToolsClient) toolNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.tools))
	for _, schema := range c.tools {
		names = append(names, schema.Function.Name)
	}
	return names
}

func (c *captureToolsClient) systemPrompt() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.messages {
		if m.Role == "system" {
			return m.Content
		}
	}
	return ""
}

type fakeNotifier struct {
	mu     sync.Mutex
	events map[string]string
	log    []notifiedEvent
}

type notifiedEvent struct {
	Method string
	Params string
}

func (n *fakeNotifier) Notify(method string, params any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.events == nil {
		n.events = map[string]string{}
	}
	b, _ := json.Marshal(params)
	n.events[method] = string(b)
	n.log = append(n.log, notifiedEvent{Method: method, Params: string(b)})
	return nil
}

// all returns every payload seen for a method, in order.
func (n *fakeNotifier) all(method string) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []string
	for _, e := range n.log {
		if e.Method == method {
			out = append(out, e.Params)
		}
	}
	return out
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
	manuscriptIndexer := manuscript.NewIndexer(st.DB())
	nodes.SetManuscriptIndexer(manuscriptIndexer)
	pb := plot.NewBuilder(nodes, beats, threads)
	notif := &fakeNotifier{}
	svc := NewService(t.TempDir(), projects, threads, entities, rels, pb, notif,
		func(ai.ResolvedProvider) (llm.Client, error) { return client, nil }, fixedProvider("claude-code-cli"), "",
		nodes, beats).
		WithManuscript(manuscript.NewSearcher(st.DB(), nodes, manuscriptIndexer))
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
	if !strings.Contains(notif.get("companion.done"), `"project_id":"`+projectID+`"`) {
		t.Fatalf("done missing project_id: %s", notif.get("companion.done"))
	}
	if !strings.Contains(notif.get("companion.proposal"), "\"valid\":true") {
		t.Fatalf("expected valid proposal: %s", notif.get("companion.proposal"))
	}
	if !strings.Contains(notif.get("companion.proposal"), `"project_id":"`+projectID+`"`) {
		t.Fatalf("proposal missing project_id: %s", notif.get("companion.proposal"))
	}
	msgs, err := svc.History(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("transcript = %+v", msgs)
	}
}

func TestSend_PersistsSceneScopedHistory(t *testing.T) {
	client := &fakeClient{responses: []string{"장면 작업을 도와드릴게요."}}
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
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
		nodes, beats).WithHistory(NewHistoryRepo(st.DB()))
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	scene, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 1", "식탁 위 고지서", 100)
	if err != nil {
		t.Fatal(err)
	}

	runID, err := svc.SendWithCompanionOptionsAndImages(ctx, p.ID, scene.ID, "이 씬 도와줘", SendOptions{Scope: HistoryViewScene}, nil, func() int64 { return 1000 })
	if err != nil {
		t.Fatalf("Send err=%v", err)
	}
	waitFor(t, notif, "companion.done")

	msgs, err := svc.HistoryView(ctx, HistoryQuery{ProjectID: p.ID, NodeID: scene.ID, Scope: HistoryViewScene, Limit: 20})
	if err != nil {
		t.Fatalf("HistoryView: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("scene history len = %d, want 2: %+v", len(msgs), msgs)
	}
	for _, msg := range msgs {
		if msg.NodeID != scene.ID {
			t.Fatalf("history node_id = %q, want %q", msg.NodeID, scene.ID)
		}
		if msg.NodeLabel != "식탁 위 고지서" {
			t.Fatalf("node label = %q, want title", msg.NodeLabel)
		}
		if msg.Scope != HistoryScopeScene {
			t.Fatalf("scope = %q, want scene", msg.Scope)
		}
		if msg.RunID != runID {
			t.Fatalf("run_id = %q, want %q", msg.RunID, runID)
		}
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" || msgs[1].Content != "장면 작업을 도와드릴게요." {
		t.Fatalf("unexpected history messages: %+v", msgs)
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

func TestSendWithContextAndImages_AttachesLatestUserMessageBlocks(t *testing.T) {
	client := &captureMessagesClient{}
	svc, notif, projectID := newSvcWithClient(t, client)
	imageData := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})

	runID, err := svc.SendWithContextAndImages(context.Background(), projectID, "", "이 장면 이미지를 참고해줘", storycontext.DefaultContextSelection(), []ImageAttachment{{
		Name:      "scene.png",
		MediaType: "image/png",
		Data:      imageData,
		Size:      3,
	}}, func() int64 { return 1 })
	if err != nil || runID == "" {
		t.Fatalf("SendWithContextAndImages err=%v runID=%q", err, runID)
	}
	waitFor(t, notif, "companion.done")

	client.mu.Lock()
	calls := append([][]llm.ChatMessage(nil), client.messages...)
	client.mu.Unlock()
	if len(calls) == 0 {
		t.Fatal("expected one LLM chat call")
	}
	messages := calls[0]
	if len(messages) == 0 {
		t.Fatal("expected chat messages")
	}
	last := messages[len(messages)-1]
	if last.Role != "user" {
		t.Fatalf("last role = %q, want user", last.Role)
	}
	if len(last.ContentBlocks) != 2 {
		t.Fatalf("content blocks = %+v, want text + image", last.ContentBlocks)
	}
	if last.ContentBlocks[0].Type != "text" || !strings.Contains(last.ContentBlocks[0].Text, "이미지") {
		t.Fatalf("text block = %+v", last.ContentBlocks[0])
	}
	image := last.ContentBlocks[1]
	if image.Type != "image" || image.MediaType != "image/png" || image.Data != imageData {
		t.Fatalf("image block = %+v", image)
	}
}

func TestSend_UsesCurrentSceneHistoryForPromptReplay(t *testing.T) {
	client := &captureMessagesClient{}
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
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
	history := NewHistoryRepo(st.DB())
	svc := NewService(t.TempDir(), projects, threads, entities, rels, pb, notif,
		func(ai.ResolvedProvider) (llm.Client, error) { return client, nil }, fixedProvider("claude-code-cli"), "",
		nodes, beats).WithHistory(history)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	sceneA, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 1", "식탁 위 고지서", 100)
	if err != nil {
		t.Fatal(err)
	}
	sceneB, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 2", "퇴근 선언", 110)
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Append(ctx, HistoryMessage{
		ProjectID: p.ID, NodeID: sceneA.ID, RunID: "ra", Role: "assistant", Scope: HistoryScopeScene,
		Status: HistoryStatusDone, Content: "씬 A에서 제안한 현재 씬 본문 작성 선택지", CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := history.Append(ctx, HistoryMessage{
		ProjectID: p.ID, NodeID: sceneB.ID, RunID: "rb", Role: "assistant", Scope: HistoryScopeScene,
		Status: HistoryStatusDone, Content: "씬 B에서 제안한 다른 장면 선택지", CreatedAt: 1100,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SendWithCompanionOptionsAndImages(ctx, p.ID, sceneA.ID, "이전 대화 참고해줘", SendOptions{
		Scope:  HistoryViewScene,
		Intent: RequestIntent{Kind: "chat"},
	}, nil, func() int64 { return 2000 }); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notif, "companion.done")

	client.mu.Lock()
	calls := append([][]llm.ChatMessage(nil), client.messages...)
	client.mu.Unlock()
	if len(calls) == 0 {
		t.Fatal("expected one LLM chat call")
	}
	var prompt string
	for _, msg := range calls[0] {
		prompt += "\n" + msg.Content
	}
	if !strings.Contains(prompt, "씬 A에서 제안한 현재 씬 본문 작성 선택지") {
		t.Fatalf("prompt missing current scene history:\n%s", prompt)
	}
	if strings.Contains(prompt, "씬 B에서 제안한 다른 장면 선택지") {
		t.Fatalf("prompt included other scene history:\n%s", prompt)
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

func TestHistoryPersistsAcrossServiceRestart(t *testing.T) {
	home := t.TempDir()
	projectID := "project-stable"
	firstStore := session.NewStore(home)
	first, err := firstStore.EnsureWorker(projectID)
	if err != nil {
		t.Fatal(err)
	}
	at := time.UnixMilli(1000)
	if err := session.AppendMessage(firstStore.TranscriptPath(first.ID), session.Message{Role: "user", Content: "앱 종료 전 질문", Timestamp: at}); err != nil {
		t.Fatal(err)
	}
	if err := session.AppendMessage(firstStore.TranscriptPath(first.ID), session.Message{Role: "assistant", Content: "앱 재시작 후 복원될 답", Timestamp: at}); err != nil {
		t.Fatal(err)
	}

	secondStore := session.NewStore(home)
	second, err := secondStore.EnsureWorker(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("worker session was not reused across restart: first=%s second=%s", first.ID, second.ID)
	}
	msgs, err := session.ReadMessages(secondStore.TranscriptPath(second.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Content != "앱 종료 전 질문" || msgs[1].Content != "앱 재시작 후 복원될 답" {
		t.Fatalf("history not restored after restart: %+v", msgs)
	}
}

func TestHistoryViewImportsLongLegacyTranscriptLine(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(home, "library.db"))
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
	svc := NewService(filepath.Join(home, "companion"), projects, threads, entities, rels, pb, notif,
		func(ai.ResolvedProvider) (llm.Client, error) { return &fakeClient{}, nil }, fixedProvider("claude-code-cli"), home,
		nodes, beats).WithHistory(NewHistoryRepo(st.DB()))
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "t", Genres: []string{"f"}, LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}

	sess, err := svc.sessions.EnsureWorker(p.ID)
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	longReply := strings.Repeat("긴 원고 응답입니다. ", 5000)
	if len(longReply) <= 64*1024 {
		t.Fatalf("test fixture too short: %d", len(longReply))
	}
	if err := session.AppendMessage(svc.sessions.TranscriptPath(sess.ID), session.Message{
		Role:      "assistant",
		Content:   longReply,
		Timestamp: time.UnixMilli(1000).UTC(),
	}); err != nil {
		t.Fatalf("append long legacy message: %v", err)
	}

	msgs, err := svc.HistoryView(ctx, HistoryQuery{ProjectID: p.ID, Scope: HistoryViewProject, Limit: 10})
	if err != nil {
		t.Fatalf("HistoryView: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != longReply {
		t.Fatalf("long legacy message not imported: len=%d", len(msgs))
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
	if got := notif.get("companion.applied"); !strings.Contains(got, `"project_id":"`+projectID+`"`) {
		t.Fatalf("applied event missing project_id: %s", got)
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

func TestSend_SceneTextApplyUsesVerifiedAppMessage(t *testing.T) {
	client := &sceneTextApplyClient{}
	svc, notif, projectID := newSvcWithClient(t, client)
	proj, err := svc.projects.Get(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := *proj.LastOpenedNodeID

	runID, err := svc.Send(context.Background(), projectID, nodeID, "아니 1장 1씬 작성해달라고", func() int64 { return 1000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	waitFor(t, notif, "companion.done")

	if got := notif.get("companion.applied"); !strings.Contains(got, `"changed_nodes"`) || !strings.Contains(got, nodeID) {
		t.Fatalf("applied event should include changed scene metadata: %s", got)
	}
	if got := notif.get("companion.done"); !strings.Contains(got, "현재 씬 본문을 반영했습니다") {
		t.Fatalf("scene edit success should use verified app message, got %s", got)
	}
	if got := notif.get("companion.done"); !strings.Contains(got, "작업 흐름") ||
		!strings.Contains(got, "요청 처리: 현재 씬 작성") ||
		!strings.Contains(got, "시작 부분") ||
		!strings.Contains(got, "새 원고 첫 문장") {
		t.Fatalf("scene edit success should include natural apply summary, got %s", got)
	}
	n, err := svc.nodes.Get(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n.ContentDoc == nil || !strings.Contains(*n.ContentDoc, "새 원고 첫 문장") {
		t.Fatalf("scene content was not applied: %+v", n)
	}

	client.mu.Lock()
	sawToolResult := client.sawToolResult
	client.mu.Unlock()
	if !sawToolResult {
		t.Fatal("expected final model turn to receive changed_nodes tool result")
	}
}

func TestSend_SceneTextFollowupApprovalAppliesCurrentScene(t *testing.T) {
	client := &sceneTextApplyClient{}
	svc, notif, projectID := newSvcWithClient(t, client)
	proj, err := svc.projects.Get(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := *proj.LastOpenedNodeID
	sess, err := svc.sessions.EnsureWorker(projectID)
	if err != nil {
		t.Fatal(err)
	}
	path := svc.sessions.TranscriptPath(sess.ID)
	at := time.UnixMilli(1000)
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: "1번", Timestamp: at}); err != nil {
		t.Fatal(err)
	}
	if err := session.AppendMessage(path, session.Message{Role: "assistant", Content: "원하시면 제가 바로 현재 씬 본문을 완성해서 적용할 수 있습니다.", Timestamp: at}); err != nil {
		t.Fatal(err)
	}

	runID, err := svc.Send(context.Background(), projectID, nodeID, "적용해줘", func() int64 { return 2000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	waitFor(t, notif, "companion.done")

	if got := notif.get("companion.done"); !strings.Contains(got, "현재 씬 본문을 반영했습니다") {
		t.Fatalf("followup scene approval should use verified app message, got %s", got)
	}
	if got := notif.get("companion.done"); !strings.Contains(got, "작업 흐름") {
		t.Fatalf("followup scene approval should include apply summary, got %s", got)
	}
	n, err := svc.nodes.Get(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n.ContentDoc == nil || !strings.Contains(*n.ContentDoc, "새 원고 첫 문장") {
		t.Fatalf("scene followup did not apply content: %+v", n)
	}
}

func TestSend_SceneTextProposalFallbackAppliesAfterToolCallFailure(t *testing.T) {
	client := &fakeClient{responses: []string{
		"현재 씬 본문을 반영했습니다.",
		"```linetta-proposal\n{\"summary\":\"현재 씬 작성\",\"ops\":[{\"op\":\"set_scene_text\",\"text\":\"새 원고 첫 문장\\n새 원고 둘째 문장\"}]}\n```",
	}}
	svc, notif, projectID := newSvcWithClient(t, client)
	proj, err := svc.projects.Get(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := *proj.LastOpenedNodeID

	runID, err := svc.Send(context.Background(), projectID, nodeID, "현재 씬 본문 써줘", func() int64 { return 1000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	waitFor(t, notif, "companion.done")

	if got := notif.get("companion.error"); got != "" {
		t.Fatalf("fallback should not emit error: %s", got)
	}
	if got := notif.get("companion.applied"); !strings.Contains(got, `"applied":1`) || !strings.Contains(got, nodeID) {
		t.Fatalf("expected fallback to apply proposal ops: %s", got)
	}
	n, err := svc.nodes.Get(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n.ContentDoc == nil || !strings.Contains(*n.ContentDoc, "새 원고 첫 문장") {
		t.Fatalf("scene proposal fallback did not apply content: %+v", n)
	}
}

func TestSend_SceneTextQuestionOnlyFailsInsteadOfClaimingDone(t *testing.T) {
	client := &sceneQuestionOnlyClient{}
	svc, notif, projectID := newSvcWithClient(t, client)
	proj, err := svc.projects.Get(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}

	runID, err := svc.Send(context.Background(), projectID, *proj.LastOpenedNodeID, "현재 씬 본문 써줘", func() int64 { return 1000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	waitFor(t, notif, "companion.error")

	client.mu.Lock()
	calls := client.calls
	sawCorrection := client.sawCorrection
	client.mu.Unlock()
	if calls < 2 || !sawCorrection {
		t.Fatalf("expected a corrective retry before failure, calls=%d sawCorrection=%v", calls, sawCorrection)
	}
	if got := notif.get("companion.error"); !strings.Contains(got, "본문 변경") {
		t.Fatalf("error should explain scene text was not applied: %s", got)
	}
	if got := notif.get("companion.done"); got != "" {
		t.Fatalf("question-only scene edit should not finish as done: %s", got)
	}
}

func TestSend_ApplyToolFailureThenBlankResponseDoesNotFinishDone(t *testing.T) {
	client := &malformedApplyThenBlankClient{}
	svc, notif, projectID := newSvcWithClient(t, client)

	runID, err := svc.Send(context.Background(), projectID, "", "아웃라인에 반영해줘", func() int64 { return 1000 })
	if err != nil || runID == "" {
		t.Fatalf("Send err=%v runID=%q", err, runID)
	}
	for i := 0; i < 200; i++ {
		if notif.get("companion.error") != "" || notif.get("companion.done") != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := notif.get("companion.done"); got != "" {
		t.Fatalf("tool failure followed by blank model response should not finish as done: %s", got)
	}
	if got := notif.get("companion.error"); !strings.Contains(got, "작품 변경") {
		t.Fatalf("error should explain apply failure, got %s", got)
	}

	client.mu.Lock()
	calls := client.calls
	sawToolFailure := client.sawToolFailure
	client.mu.Unlock()
	if calls < 2 || !sawToolFailure {
		t.Fatalf("expected blank final turn after tool failure, calls=%d sawToolFailure=%v", calls, sawToolFailure)
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

type sceneTextApplyClient struct {
	mu            sync.Mutex
	calls         int
	sawToolResult bool
}

func (c *sceneTextApplyClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *sceneTextApplyClient) Chat(_ context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	switch c.calls {
	case 1:
		if opts.ToolChoice == nil || opts.ToolChoice.Mode != llm.ToolChoiceModeRequired {
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "tool choice missing"}}, nil
		}
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:        "call_scene",
				Name:      applyOpsToolName,
				Arguments: `{"summary":"현재 씬 작성","ops_json":"[{\"op\":\"set_scene_text\",\"text\":\"새 원고 첫 문장\\n새 원고 둘째 문장\"}]"}`,
			}},
		}}, nil
	default:
		if len(messages) > 0 && messages[len(messages)-1].Role == "tool" && strings.Contains(messages[len(messages)-1].Content, `"changed_nodes"`) {
			c.sawToolResult = true
		}
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "적용했습니다.",
		}}, nil
	}
}

type sceneQuestionOnlyClient struct {
	mu            sync.Mutex
	calls         int
	sawCorrection bool
}

func (c *sceneQuestionOnlyClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *sceneQuestionOnlyClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if len(messages) > 0 && strings.Contains(messages[len(messages)-1].Content, "실제 작품 상태 변경") {
		c.sawCorrection = true
	}
	return llm.ChatResponse{Message: llm.ChatMessage{
		Role:    "assistant",
		Content: "어떤 분위기로 쓰면 될까요?",
	}}, nil
}

type malformedApplyThenBlankClient struct {
	mu             sync.Mutex
	calls          int
	sawToolFailure bool
}

func (c *malformedApplyThenBlankClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *malformedApplyThenBlankClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	switch c.calls {
	case 1:
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:        "call_bad_apply",
				Name:      applyOpsToolName,
				Arguments: `{"summary":"아웃라인 반영","ops_json":"[{\"op\":\"set_outline\",\"outline\":\"새 개요\"} invalid]"}`,
			}},
		}}, nil
	default:
		if len(messages) > 0 && messages[len(messages)-1].Role == "tool" && strings.Contains(messages[len(messages)-1].Content, "invalid JSON") {
			c.sawToolFailure = true
		}
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role:    "assistant",
			Content: "",
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

	msgs, err := svc.CompactHistory(context.Background(), projectID, "", func() int64 { return 2000 })
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

func TestGatherContext_IncludesWrittenSceneText(t *testing.T) {
	svc, _, projectID := newSvc(t, "안녕")
	ctx := context.Background()
	p, err := svc.projects.Get(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if p.LastOpenedNodeID == nil {
		t.Fatal("project has no LastOpenedNodeID")
	}
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"해진은 민호와 비밀 동맹을 맺었다."}]}]}`
	if err := svc.nodes.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 1200); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	d, err := svc.gatherContext(ctx, projectID, "", "작성된 글을 분석해서 캐릭터와 관계 구성해줘")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.SceneExcerpts) == 0 {
		t.Fatal("expected scene excerpts in companion context")
	}
	if !strings.Contains(d.SceneExcerpts[0].Text, "비밀 동맹") {
		t.Fatalf("scene excerpt missing written body: %+v", d.SceneExcerpts)
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

func TestApplyContextSelection_RemovesUncheckedCompanionSections(t *testing.T) {
	off := false
	data := PromptData{
		Outline: "자의식에 관한 작품 개요",
		SceneExcerpts: []SceneExcerpt{{
			NodeID: "n1",
			Label:  "씬 1",
			Text:   "인간의 개별성은 무엇일까?",
		}},
		Threads: []thread.Thread{{ID: "t1", Name: "자의식의 균열"}},
		Entities: []entity.Entity{
			{ID: "e1", Kind: "character", Name: "해진"},
			{ID: "e2", Kind: "place", Name: "거울방"},
		},
		Relationships: []relationship.Relationship{{
			FromID: "e1",
			ToID:   "e2",
			Label:  "집착",
		}},
		Facts: []fact.Card{{
			ID:     "f1",
			Claim:  "자의식은 자기 인식과 관련된다",
			Result: "검증된 참고",
			Status: fact.StatusVerified,
		}},
		Memories: []string{"작가는 철학적인 질문을 좋아한다"},
	}

	selection := storycontext.ContextSelection{
		CurrentScene:  &off,
		Overview:      &off,
		Plot:          &off,
		Entities:      &off,
		Relationships: &off,
		Facts:         &off,
		Memories:      &off,
	}

	text := buildContext(applyContextSelection(data, selection), "")
	for _, blocked := range []string{
		"자의식에 관한 작품 개요",
		"인간의 개별성",
		"자의식의 균열",
		"해진",
		"집착",
		"자의식은 자기 인식",
		"철학적인 질문",
	} {
		if strings.Contains(text, blocked) {
			t.Fatalf("unchecked context %q still rendered in:\n%s", blocked, text)
		}
	}
}

func TestPreviewFromPromptData_RendersSelectableCompanionSections(t *testing.T) {
	off := false
	data := PromptData{
		Outline: "자의식을 다루는 소설",
		SceneExcerpts: []SceneExcerpt{{
			NodeID:    "n1",
			Label:     "씬 1",
			Text:      "인간의 개별성은 무엇일까?",
			IsCurrent: true,
		}},
		Facts: []fact.Card{{
			ID:       "f1",
			Claim:    "일반 경찰은 통상 비무장이다",
			Result:   "배경 참고",
			Status:   fact.StatusVerified,
			Category: "reference",
			Sources:  []fact.Source{{URL: "https://example.com", Title: "자료"}},
		}},
		Memories: []string{"작가는 모호한 결말을 선호한다"},
	}

	preview := previewFromPromptData(data, storycontext.ContextSelection{Facts: &off})

	var sawScene, sawFact bool
	for _, section := range preview.Sections {
		if section.ID == storycontext.ContextKeyCurrentScene {
			sawScene = true
			if !section.Selected || !strings.Contains(section.Preview, "인간의 개별성") {
				t.Fatalf("scene preview not selected/rendered: %+v", section)
			}
		}
		if section.ID == storycontext.ContextKeyFacts {
			sawFact = true
			if section.Selected || !strings.Contains(section.Preview, "일반 경찰") {
				t.Fatalf("facts preview should be visible but unselected: %+v", section)
			}
		}
	}
	if !sawScene || !sawFact {
		t.Fatalf("missing expected sections in preview: %+v", preview.Sections)
	}
	if preview.SelectedTokenEstimate == 0 || preview.BudgetTokenEstimate == 0 {
		t.Fatalf("expected token estimates in preview: %+v", preview)
	}
}

func TestBuildContext_RendersReferencesByPurpose(t *testing.T) {
	data := PromptData{
		References: []Reference{
			{
				ID:      "r1",
				Purpose: ReferencePurposeStyle,
				Title:   "자서전 문체",
				Content: "그는 한참 뒤에야 그 문장을 이해했다.",
				Status:  ReferenceStatusActive,
			},
			{
				ID:      "r2",
				Purpose: ReferencePurposeConstraint,
				Title:   "금지 톤",
				Content: "가족을 악인처럼 단정하지 않는다.",
				Status:  ReferenceStatusActive,
			},
		},
	}
	text := buildContext(data, "")
	for _, want := range []string{
		"## 추가 레퍼런스",
		"문체 참고",
		"문장 리듬",
		"고유 표현을 그대로 복사하지 마세요",
		"금지/주의",
		"가족을 악인처럼 단정하지 않는다",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("context missing %q:\n%s", want, text)
		}
	}
}

func TestApplyContextSelection_RemovesReferences(t *testing.T) {
	off := false
	data := PromptData{
		References: []Reference{{
			ID:      "r1",
			Purpose: ReferencePurposeContent,
			Title:   "참고",
			Content: "프롬프트에 들어가면 안 되는 레퍼런스",
			Status:  ReferenceStatusActive,
		}},
	}
	text := buildContext(applyContextSelection(data, storycontext.ContextSelection{References: &off}), "")
	if strings.Contains(text, "프롬프트에 들어가면 안 되는") {
		t.Fatalf("unchecked reference still rendered:\n%s", text)
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

// Regression for the reported bug: "diagnose it, do not change it" turns must
// not even be offered the mutation tool, so no remember/settings op can run.
func TestSend_ReadOnlyDiagnosisWithholdsMutationTool(t *testing.T) {
	client := &captureToolsClient{reply: "1화 초안 진단 결과입니다. 문체는 안정적이고, 개연성은 후반부에서 약해집니다."}
	svc, notif, projectID := newSvcWithClient(t, client)

	_, err := svc.Send(context.Background(), projectID, "",
		"현재 1화 초안을 수정하지 말고 먼저 진단해줘. 문체와 개연성을 평가하고 수정 제안만 제시해줘.",
		func() int64 { return 1000 })
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, notif, "companion.done")

	names := strings.Join(client.toolNames(), ",")
	if strings.Contains(names, applyOpsToolName) {
		t.Fatalf("read-only diagnosis was offered the mutation tool: %q", names)
	}
	if !strings.Contains(client.systemPrompt(), "읽기 전용 요청입니다") {
		t.Fatal("system prompt should carry the read-only turn instruction")
	}
	if !strings.Contains(notif.get("companion.done"), `"intent":"read_only"`) {
		t.Fatalf("done payload should report the read-only intent: %s", notif.get("companion.done"))
	}
	if recalled := svc.Recall(projectID, "문체", 5); len(recalled) != 0 {
		t.Fatalf("read-only diagnosis wrote work memory: %v", recalled)
	}
}

// phasesFrom pulls the phase names out of companion.thinking payloads in order,
// collapsing repeats so the sequence reads like the UI stepper.
func phasesFrom(t *testing.T, payloads []string) []string {
	t.Helper()
	var out []string
	for _, raw := range payloads {
		var p struct {
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("thinking payload: %v", err)
		}
		if p.Phase == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == p.Phase {
			continue
		}
		out = append(out, p.Phase)
	}
	return out
}

func TestSend_ReportsRequestAndGenerationPhases(t *testing.T) {
	svc, notif, projectID := newSvc(t, "이어질 문장을 제안합니다.")

	if _, err := svc.Send(context.Background(), projectID, "", "다음 전개 아이디어 줘", func() int64 { return 1000 }); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, notif, "companion.done")

	got := phasesFrom(t, notif.all("companion.thinking"))
	if len(got) < 2 || got[0] != phaseRequesting || got[1] != phaseGenerating {
		t.Fatalf("phase sequence = %v, want requesting then generating", got)
	}
}

func TestSend_ReportsVerifyApplyPhasesWithCounts(t *testing.T) {
	client := &claimThenApplyClient{}
	svc, notif, projectID := newSvcWithClient(t, client)

	if _, err := svc.Send(context.Background(), projectID, "", "작품 전체 아웃라인 구성해줘", func() int64 { return 1000 }); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, notif, "companion.done")

	payloads := notif.all("companion.thinking")
	got := phasesFrom(t, payloads)
	for _, want := range []string{phaseRequesting, phaseVerifying, phaseApplying, phaseApplied} {
		if !containsString(got, want) {
			t.Fatalf("phase sequence %v missing %q", got, want)
		}
	}
	if indexOfString(got, phaseVerifying) > indexOfString(got, phaseApplying) {
		t.Fatalf("verify should come before apply: %v", got)
	}
	if !hasPayload(payloads, `"phase":"applying"`, `"total":2`) {
		t.Fatalf("applying phase should carry the op count: %v", payloads)
	}
	if !hasPayload(payloads, `"phase":"applied"`, `"applied":2`) {
		t.Fatalf("applied phase should carry how many ops landed: %v", payloads)
	}
}

func TestSend_StallTimeoutReportsRetryableError(t *testing.T) {
	restore := shortenStallWatchdog(t)
	defer restore()

	client := &blockingClient{released: make(chan struct{})}
	svc, notif, projectID := newSvcWithClient(t, client)

	if _, err := svc.Send(context.Background(), projectID, "", "긴 아웃라인 작업 해줘", func() int64 { return 1000 }); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, notif, "companion.error")

	got := notif.get("companion.error")
	if !strings.Contains(got, "다시 시도") {
		t.Fatalf("stalled run should fail with a retryable message: %s", got)
	}
	if notif.get("companion.cancelled") != "" {
		t.Fatalf("a stall is not a user cancel: %s", notif.get("companion.cancelled"))
	}
}

func shortenStallWatchdog(t *testing.T) func() {
	t.Helper()
	prevTimeout, prevInterval := companionStallTimeout, companionStallCheckInterval
	companionStallTimeout = 60 * time.Millisecond
	companionStallCheckInterval = 10 * time.Millisecond
	return func() {
		companionStallTimeout, companionStallCheckInterval = prevTimeout, prevInterval
	}
}

// blockingClient never answers until the run context is cancelled, standing in
// for a provider that accepted the request and went silent.
type blockingClient struct {
	released chan struct{}
}

func (c *blockingClient) Ask(context.Context, string) (string, error) { return "", nil }
func (c *blockingClient) Chat(ctx context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	select {
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	case <-c.released:
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "늦은 응답"}}, nil
	}
}

func containsString(values []string, want string) bool {
	return indexOfString(values, want) >= 0
}

func indexOfString(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return -1
}

func hasPayload(payloads []string, needles ...string) bool {
	for _, raw := range payloads {
		matched := true
		for _, needle := range needles {
			if !strings.Contains(raw, needle) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// bigOutlineApplyClient asks to build a whole book's outline in one tool call.
type bigOutlineApplyClient struct {
	mu    sync.Mutex
	calls int
}

func (c *bigOutlineApplyClient) Ask(context.Context, string) (string, error) { return "", nil }
func (c *bigOutlineApplyClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls == 1 {
		return llm.ChatResponse{Message: llm.ChatMessage{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:        "call_outline",
				Name:      applyOpsToolName,
				Arguments: `{"summary":"전체 아웃라인","ops_json":"` + bigOutlineOpsEscaped + `"}`,
			}},
		}}, nil
	}
	return llm.ChatResponse{Message: llm.ChatMessage{
		Role:    "assistant",
		Content: "구조 변경안을 준비했습니다. 확인 후 적용해 주세요.",
	}}, nil
}

// Eight outline creations: past the point where the writer gets a look first.
const bigOutlineOpsEscaped = `[{\"op\":\"create_outline_node\",\"ref\":\"p1\",\"kind\":\"container\",\"label\":\"1부\"},` +
	`{\"op\":\"create_outline_node\",\"ref\":\"s1\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 1\"},` +
	`{\"op\":\"create_outline_node\",\"ref\":\"s2\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 2\"},` +
	`{\"op\":\"create_outline_node\",\"ref\":\"s3\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 3\"},` +
	`{\"op\":\"create_outline_node\",\"ref\":\"s4\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 4\"},` +
	`{\"op\":\"create_outline_node\",\"ref\":\"s5\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 5\"},` +
	`{\"op\":\"create_outline_node\",\"ref\":\"s6\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 6\"},` +
	`{\"op\":\"create_outline_node\",\"ref\":\"s7\",\"kind\":\"leaf\",\"parent_node_ref\":\"p1\",\"label\":\"씬 7\"}]`

// A whole-book restructure is shown to the writer instead of landing silently,
// and the run still finishes normally.
func TestSend_LargeOutlineChangeWaitsForApproval(t *testing.T) {
	client := &bigOutlineApplyClient{}
	svc, notif, projectID := newSvcWithClient(t, client)
	before, err := svc.nodes.ListByProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}

	if _, err := svc.Send(context.Background(), projectID, "", "작품 전체 아웃라인 구성해줘", func() int64 { return 1000 }); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitFor(t, notif, "companion.done")

	preview := notif.get("companion.preview")
	if preview == "" {
		t.Fatal("expected the change to be offered as a preview")
	}
	if !strings.Contains(preview, `"created":8`) {
		t.Fatalf("preview should count what it would create: %s", preview)
	}
	if got := notif.get("companion.error"); got != "" {
		t.Fatalf("waiting on the writer is not a failure: %s", got)
	}
	if got := notif.get("companion.applied"); got != "" {
		t.Fatalf("nothing should have been applied yet: %s", got)
	}

	after, err := svc.nodes.ListByProject(context.Background(), projectID)
	if err != nil {
		t.Fatalf("nodes.ListByProject: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("outline changed before approval: %d nodes before, %d after", len(before), len(after))
	}
}
