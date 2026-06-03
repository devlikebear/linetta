package companion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/streamdedup"
	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/google/uuid"
)

type deltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}
type resetPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}
type donePayload struct {
	RunID    string `json:"run_id"`
	FullText string `json:"full_text"`
}
type errorPayload struct {
	RunID   string `json:"run_id"`
	Message string `json:"message"`
}
type cancelledPayload struct {
	RunID string `json:"run_id"`
}
type proposalPayload struct {
	RunID   string `json:"run_id"`
	Valid   bool   `json:"valid"`
	Summary string `json:"summary,omitempty"`
	Ops     []Op   `json:"ops,omitempty"`
	Error   string `json:"error,omitempty"`
}
type appliedPayload struct {
	RunID   string `json:"run_id"`
	Summary string `json:"summary,omitempty"`
	Applied int    `json:"applied"`
}
type choicesPayload struct {
	RunID       string   `json:"run_id"`
	Prompt      string   `json:"prompt,omitempty"`
	Options     []string `json:"options,omitempty"`
	AllowCustom bool     `json:"allow_custom"`
}
type thinkingPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}
type reasoningPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}

// friendlyToolLabel maps a tool name to a human-readable status shown while the
// companion is working, so the user sees what the AI is doing.
func friendlyToolLabel(name string) string {
	switch name {
	case "web_search":
		return "웹 검색 중…"
	case "web_fetch":
		return "웹 페이지 읽는 중…"
	case "linetta_apply_ops":
		return "작품 설정 반영 중…"
	default:
		return "도구 실행 중: " + name
	}
}

// Runner manages companion run lifecycle + cancellation.
type Runner struct {
	svc    *Service
	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func newRunner(svc *Service) *Runner {
	return &Runner{svc: svc, active: map[string]context.CancelFunc{}}
}

func (r *Runner) start(ctx context.Context, projectID, nodeID, text string, now func() int64) (string, error) {
	sess, err := r.svc.sessions.EnsureWorker(projectID)
	if err != nil {
		return "", err
	}
	path := r.svc.sessions.TranscriptPath(sess.ID)

	data, err := r.svc.gatherContext(ctx, projectID, nodeID, text)
	if err != nil {
		return "", err
	}

	// Persist the user turn before streaming so transcript failures are visible
	// before the assistant starts generating against missing history.
	userAt := now()
	if err := session.AppendMessage(path, session.Message{Role: "user", Content: text, Timestamp: time.UnixMilli(userAt)}); err != nil {
		r.svc.recordPersistenceError(ctx, userAt, "user", path, err)
		return "", fmt.Errorf("companion transcript: %w", err)
	}
	r.svc.recordPersistenceOK(ctx, userAt, "user", path)

	// Build the message list: system + context + history (history already
	// includes the just-appended user turn as its last item).
	msgs := []llm.ChatMessage{{Role: "system", Content: buildSystem()}}
	if cctx := buildContext(data); cctx != "" {
		msgs = append(msgs, llm.ChatMessage{Role: "user", Content: cctx})
	}
	if hist, err := session.LoadHistory(path, historyTokenBudget); err == nil {
		for _, m := range hist {
			msgs = append(msgs, llm.ChatMessage{Role: m.Role, Content: m.Content})
		}
	}

	rp := r.svc.src.Resolve()
	rp.WorkDir = r.svc.workDir
	client, err := r.svc.factory(rp)
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runID := uuid.NewString()
	r.mu.Lock()
	r.active[runID] = cancel
	r.mu.Unlock()

	go r.run(runCtx, runID, projectID, nodeID, path, text, msgs, client, now)
	return runID, nil
}

func (r *Runner) finish(runID string) {
	r.mu.Lock()
	if c, ok := r.active[runID]; ok {
		c()
		delete(r.active, runID)
	}
	r.mu.Unlock()
}

func (r *Runner) cancel(runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.active[runID]
	if !ok {
		return errors.New("companion: run not found or already finished")
	}
	c()
	delete(r.active, runID)
	return nil
}

const maxQueryRounds = 3
const applyOpsToolName = "linetta_apply_ops"

func (r *Runner) run(ctx context.Context, runID, projectID, nodeID, path, userText string, msgs []llm.ChatMessage, client llm.Client, now func() int64) {
	defer r.finish(runID)

	registry := r.svc.buildToolRegistry(projectID, nodeID, now, runID, userText)
	dedup := streamdedup.New()
	queryRounds := 0
	forcedTool := companionForcedToolForUserText(userText)
	forcedApplyOps := forcedTool == applyOpsToolName
	applyOpsSucceeded := false
	applyOpsCorrectionUsed := false
	client = newFirstTurnToolChoiceClient(client, forcedTool)
	loop := agentloop.New(client, registry, agentloop.HookFunc(func(ctx context.Context, evt agentloop.Event) {
		switch evt.Type {
		case agentloop.EventBeforeTool:
			_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, Text: friendlyToolLabel(evt.ToolName)})
		case agentloop.EventAfterTool:
			if evt.ToolName == "linetta_apply_ops" && !evt.ToolIsError {
				applyOpsSucceeded = true
				_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, Text: "작품 설정을 갱신했습니다"})
			}
		}
	}))
	resp, err := loop.Run(ctx, msgs, agentloop.RunOptions{
		MaxIterations: 8,
		Tools:         registry.Schemas(),
		OnDelta: func(text string) {
			switch act, payload := dedup.Observe(text); act {
			case streamdedup.ActionEmit:
				_ = r.svc.notify.Notify("companion.delta", deltaPayload{RunID: runID, Text: payload})
			case streamdedup.ActionReset:
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, Text: payload})
			case streamdedup.ActionSkip:
			}
		},
		OnReasoningDelta: func(text string) {
			if strings.TrimSpace(text) == "" {
				return
			}
			_ = r.svc.notify.Notify("companion.reasoning", reasoningPayload{RunID: runID, Text: text})
		},
		OnTurnEnd: func(ctx context.Context, lastResp llm.ChatResponse) (string, error) {
			if forcedApplyOps && !applyOpsSucceeded && !applyOpsCorrectionUsed {
				applyOpsCorrectionUsed = true
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, Text: ""})
				_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, Text: friendlyToolLabel(applyOpsToolName)})
				dedup = streamdedup.New()
				return directApplyCorrectionPrompt(userText), nil
			}
			if queryRounds >= maxQueryRounds-1 {
				return "", nil
			}
			full := lastResp.Message.Content
			if qr, present, qerr := ParseQuery(full); present && qerr == nil {
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, Text: ""})
				_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, Text: querySummary(qr)})
				queryRounds++
				dedup = streamdedup.New()
				return r.svc.runQueries(ctx, projectID, qr.Queries), nil
			}
			return "", nil
		},
	})
	if ctx.Err() != nil {
		_ = r.svc.notify.Notify("companion.cancelled", cancelledPayload{RunID: runID})
		return
	}
	if err != nil {
		_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, Message: err.Error()})
		return
	}
	full := resp.Message.Content
	if full == "" {
		full = dedup.Final()
	}

	assistantAt := now()
	if err := session.AppendMessage(path, session.Message{Role: "assistant", Content: full, Timestamp: time.UnixMilli(assistantAt)}); err != nil {
		r.svc.recordPersistenceError(ctx, assistantAt, "assistant", path, err)
		_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, Message: "companion transcript: " + err.Error()})
		return
	}
	r.svc.recordPersistenceOK(ctx, assistantAt, "assistant", path)
	if prop, present, perr := ParseProposal(full); present {
		pp := proposalPayload{RunID: runID, Valid: perr == nil, Summary: prop.Summary, Ops: prop.Ops}
		if perr != nil {
			pp.Error = perr.Error()
			pp.Ops = nil
		}
		_ = r.svc.notify.Notify("companion.proposal", pp)
	}
	// A valid choices block becomes an interactive button list. Malformed blocks
	// are dropped silently (no card) so the writer just sees the prose.
	if ch, present, cerr := ParseChoices(full); present && cerr == nil {
		_ = r.svc.notify.Notify("companion.choices", choicesPayload{
			RunID:       runID,
			Prompt:      ch.Prompt,
			Options:     ch.Options,
			AllowCustom: ch.AllowCustom,
		})
	}
	_ = r.svc.notify.Notify("companion.done", donePayload{RunID: runID, FullText: full})
}

// querySummary returns a short "조회 중: toolA, toolB" status string.
func querySummary(qr QueryRequest) string {
	names := make([]string, 0, len(qr.Queries))
	for _, q := range qr.Queries {
		names = append(names, q.Tool)
	}
	return "조회 중: " + strings.Join(names, ", ")
}

func companionForcedToolForUserText(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return ""
	}
	if containsAny(s, companionEducationalTerms) {
		return ""
	}
	if containsAny(s, companionResearchTerms) {
		return ""
	}
	if containsAny(s, companionDiscussionTerms) && !containsAny(s, companionDirectApplyTerms) {
		return ""
	}
	if containsAny(s, companionStructureTerms) && containsAny(s, companionMutationTerms) {
		return applyOpsToolName
	}
	return ""
}

func directApplyCorrectionPrompt(userText string) string {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		userText = "(원문 없음)"
	}
	return "방금 사용자 요청은 설명이나 제안이 아니라 실제 작품 상태 변경 요청입니다. " +
		"변경했다고 말로만 답하지 말고 linetta_apply_ops를 호출해 작품 상태에 적용하세요. " +
		"아웃라인/목차 요청이면 create_outline_node 또는 create_scene으로 왼쪽 아웃라인 트리를 만들고, 필요한 경우 그 노드에 create_thread/add_beat를 함께 연결하세요. " +
		"현재 정보만으로 적용할 수 없으면 적용하지 말고 부족한 정보를 한 문장으로 질문하세요.\n\n" +
		"사용자 요청: " + userText
}

var companionStructureTerms = []string{
	"스토리라인", "줄거리", "플롯", "비트", "캐릭터", "인물", "관계",
	"장소", "씬", "장면", "개요", "요약", "기억", "설정", "세계관",
	"시놉시스", "아웃라인", "얼개", "구조", "챕터", "막", "파트",
	"에피소드", "회차",
}

var companionMutationTerms = []string{
	"수정", "추가", "생성", "만들", "바꿔", "변경", "반영", "저장",
	"붙여", "넣어", "정리", "업데이트", "삭제", "지워", "작성",
	"써", "짜", "구성", "잡아", "세워", "완성", "나눠", "나누",
	"쪼개", "분할", "세분", "구체화", "확장", "전개", "다듬",
	"고쳐", "재작성", "초기화", "비워", "채워",
}

var companionEducationalTerms = []string{
	"작성법", "방법", "어떻게", "가이드",
}

var companionResearchTerms = []string{
	"검색", "찾아", "조사", "웹", "web", "url", "링크", "자료", "최신",
	"레퍼런스",
}

var companionDirectApplyTerms = []string{
	"해줘", "해 줘", "해주세요", "해 주세요", "반영해", "저장해", "수정해",
	"추가해", "만들어", "넣어", "붙여", "작성해", "써줘", "써 줘",
	"짜줘", "짜 줘", "구성해", "잡아줘", "잡아 줘", "세워줘", "세워 줘",
	"나눠줘", "나눠 줘", "쪼개줘", "쪼개 줘", "구체화해", "확장해",
	"전개해", "다듬어", "고쳐줘", "고쳐 줘", "재작성해",
}

var companionDiscussionTerms = []string{
	"어때", "추천", "아이디어", "설명", "알려", "검토", "브레인스토밍",
	"가능할까", "괜찮을까",
}

func containsAny(s string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

type firstTurnToolChoiceClient struct {
	inner llm.Client
	tool  string
	mu    sync.Mutex
	used  bool
}

func newFirstTurnToolChoiceClient(inner llm.Client, toolName string) llm.Client {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return inner
	}
	return &firstTurnToolChoiceClient{inner: inner, tool: toolName}
}

func (c *firstTurnToolChoiceClient) Ask(ctx context.Context, prompt string) (string, error) {
	return c.inner.Ask(ctx, prompt)
}

func (c *firstTurnToolChoiceClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	if !c.used {
		if tools := filterToolSchemas(opts.Tools, c.tool); len(tools) > 0 {
			opts.Tools = tools
			opts.ToolChoice = llm.ToolChoiceRequired()
		}
		c.used = true
	}
	c.mu.Unlock()
	return c.inner.Chat(ctx, messages, opts)
}

func filterToolSchemas(tools []llm.ToolSchema, name string) []llm.ToolSchema {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	out := make([]llm.ToolSchema, 0, 1)
	for _, schema := range tools {
		if schema.Function.Name == name {
			out = append(out, schema)
		}
	}
	return out
}
