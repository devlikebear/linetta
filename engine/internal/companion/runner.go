package companion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/streamdedup"
	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/google/uuid"
)

type deltaPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Text      string `json:"text"`
}
type resetPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Text      string `json:"text"`
}
type donePayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	FullText  string `json:"full_text"`
}
type errorPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Message   string `json:"message"`
}
type cancelledPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	// Applied reports whether a change had already finished applying when the
	// stop arrived, so the UI can say the work was not left half-changed.
	Applied bool `json:"applied,omitempty"`
}
type proposalPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Valid     bool   `json:"valid"`
	Summary   string `json:"summary,omitempty"`
	Ops       []Op   `json:"ops,omitempty"`
	Error     string `json:"error,omitempty"`
}

// previewPayload carries a structural outline change to the writer for
// approval instead of applying it.
type previewPayload struct {
	RunID     string               `json:"run_id"`
	ProjectID string               `json:"project_id"`
	NodeID    string               `json:"node_id,omitempty"`
	Scope     string               `json:"scope,omitempty"`
	Intent    string               `json:"intent,omitempty"`
	Preview   OutlineChangePreview `json:"preview"`
}

type appliedPayload struct {
	RunID        string              `json:"run_id"`
	ProjectID    string              `json:"project_id"`
	NodeID       string              `json:"node_id,omitempty"`
	Scope        string              `json:"scope,omitempty"`
	Intent       string              `json:"intent,omitempty"`
	Summary      string              `json:"summary,omitempty"`
	Applied      int                 `json:"applied"`
	ChangedNodes []AppliedNodeChange `json:"changed_nodes,omitempty"`
	UndoBatchID  string              `json:"undo_batch_id,omitempty"`
}
type choicesPayload struct {
	RunID       string   `json:"run_id"`
	ProjectID   string   `json:"project_id"`
	NodeID      string   `json:"node_id,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Intent      string   `json:"intent,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Options     []string `json:"options,omitempty"`
	AllowCustom bool     `json:"allow_custom"`
}
type thinkingPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Text      string `json:"text"`
	// Phase names the step the run is on so the UI can show progress instead of
	// one frozen status line; Applied/Total carry op counts for apply steps.
	Phase   string `json:"phase,omitempty"`
	Applied int    `json:"applied,omitempty"`
	Total   int    `json:"total,omitempty"`
}

// Run phases surfaced to the UI. A long request walks requesting -> generating
// -> verifying -> applying, with tool lookups reported along the way.
const (
	phaseRequesting = "requesting"
	phaseGenerating = "generating"
	phaseQuerying   = "querying"
	phaseSearching  = "searching"
	phaseFetching   = "fetching"
	phaseVerifying  = "verifying"
	phaseApplying   = "applying"
	phaseApplied    = "applied"
	// phaseAwaitingApproval means a structural change is sitting in a preview,
	// waiting for the writer to apply or discard it.
	phaseAwaitingApproval = "awaiting_approval"
)

// toolPhase maps a tool call to the phase it represents. Applying ops starts
// with validation, so the apply tool reports "verifying" until the ops pass and
// it switches to "applying" itself.
func toolPhase(name string) string {
	switch name {
	case "web_search":
		return phaseSearching
	case "web_fetch":
		return phaseFetching
	case applyOpsToolName:
		return phaseVerifying
	default:
		return phaseGenerating
	}
}

type reasoningPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Text      string `json:"text"`
}

// friendlyToolLabel maps a tool name to a human-readable status shown while the
// companion is working, so the user sees what the AI is doing.
func friendlyToolLabel(name, lang string) string {
	if isJapanese(lang) {
		switch name {
		case "web_search":
			return "ウェブ検索中…"
		case "web_fetch":
			return "ウェブページを読み込み中…"
		case "linetta_apply_ops":
			return "作品設定を反映中…"
		default:
			return "ツール実行中: " + name
		}
	}
	if isEnglish(lang) {
		switch name {
		case "web_search":
			return "Searching the web…"
		case "web_fetch":
			return "Reading a web page…"
		case "linetta_apply_ops":
			return "Applying story changes…"
		default:
			return "Running tool: " + name
		}
	}
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

// appliedStatusText is the transient status shown after linetta_apply_ops
// finishes during a run.
func appliedStatusText(lang string) string {
	if isEnglish(lang) {
		return "Story state updated"
	}
	if isJapanese(lang) {
		return "作品設定を更新しました"
	}
	return "작품 설정을 갱신했습니다"
}

// requestingStatusText is the status shown between sending the request and the
// first token coming back.
func requestingStatusText(lang string) string {
	return pickLang(lang, "요청 보내는 중…", "Sending the request…", "リクエストを送信中…")
}

// generatingStatusText is the status shown once the model starts answering.
func generatingStatusText(lang string) string {
	return pickLang(lang, "응답 생성 중…", "Writing the response…", "応答を生成中…")
}

// awaitingApprovalStatusText is the status shown when a structural change is
// waiting for the writer to look at it.
func awaitingApprovalStatusText(lang string) string {
	return pickLang(lang,
		"변경 미리보기를 준비했습니다",
		"Prepared a preview of the change",
		"変更のプレビューを用意しました")
}

// applyingStatusText is the status shown while validated ops are being written
// to the project.
func applyingStatusText(lang string) string {
	return pickLang(lang, "작품에 적용하는 중…", "Applying to the work…", "作品に反映中…")
}

// timedOutMessage is the failure recorded when a run goes silent for longer
// than companionStallTimeout, so the writer gets a retryable error instead of a
// spinner that never ends.
func timedOutMessage(lang string) string {
	return pickLang(lang,
		"모델 응답이 오랫동안 없어 요청을 중단했습니다. 적용된 변경은 없습니다. 다시 시도해 주세요.",
		"The model stopped responding, so the request was aborted. Nothing was applied — please try again.",
		"モデルの応答が長時間途絶えたためリクエストを中止しました。適用された変更はありません。もう一度お試しください。")
}

// cancelledAfterApplyMessage is recorded when the stop arrives after changes
// were already written: the apply is never torn in half, so it stays.
func cancelledAfterApplyMessage(lang string) string {
	return pickLang(lang,
		"요청을 중지했습니다. 중지 전에 적용이 끝난 변경은 그대로 유지됩니다.",
		"Request stopped. Changes that finished applying before the stop are kept.",
		"リクエストを中止しました。中止前に適用が完了した変更はそのまま残ります。")
}

// cancelledMessage is the assistant history entry recorded when a run is
// cancelled by the user.
func cancelledMessage(lang string) string {
	if isEnglish(lang) {
		return "Request stopped."
	}
	if isJapanese(lang) {
		return "リクエストを中止しました。"
	}
	return "요청을 중지했습니다."
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

func (r *Runner) start(ctx context.Context, projectID, nodeID, text string, selection storycontext.ContextSelection, outlineStructure string, requestIntent RequestIntent, requestedScope string, images []ImageAttachment, language string, now func() int64) (string, error) {
	sess, err := r.svc.sessions.EnsureWorker(projectID)
	if err != nil {
		return "", err
	}
	path := r.svc.sessions.TranscriptPath(sess.ID)

	data, err := r.svc.gatherContext(ctx, projectID, nodeID, text)
	if err != nil {
		return "", err
	}
	data.OutlineStructure = strings.TrimSpace(outlineStructure)
	data = applyContextSelection(data, selection)

	runID := uuid.NewString()
	scope := turnHistoryScope(requestedScope, nodeID)
	var conversation []conversationMessage
	var promptHistory []llm.ChatMessage
	if r.svc.history != nil {
		if err := r.svc.importLegacyHistoryIfNeeded(ctx, projectID); err != nil {
			return "", err
		}
		hist, err := r.svc.history.LoadForPrompt(ctx, HistoryQuery{
			ProjectID: projectID,
			NodeID:    nodeID,
			Scope:     scope,
			Limit:     24,
		})
		if err != nil {
			return "", err
		}
		for _, m := range hist {
			promptHistory = append(promptHistory, llm.ChatMessage{Role: m.Role, Content: m.Content})
			conversation = append(conversation, conversationMessage{Role: m.Role, Content: m.Content})
		}
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
	msgs := []llm.ChatMessage{{Role: "system", Content: buildSystem(language)}}
	if cctx := buildContext(data, language); cctx != "" {
		msgs = append(msgs, llm.ChatMessage{Role: "user", Content: cctx})
	}
	if r.svc.history != nil {
		msgs = append(msgs, promptHistory...)
		current := llm.ChatMessage{Role: "user", Content: text}
		if len(images) > 0 {
			current.ContentBlocks = companionImageContentBlocks(text, images)
		}
		msgs = append(msgs, current)
	} else if hist, err := session.LoadHistory(path, historyTokenBudget); err == nil {
		for i, m := range hist {
			msg := llm.ChatMessage{Role: m.Role, Content: m.Content}
			if len(images) > 0 && i == len(hist)-1 && m.Role == "user" {
				msg.ContentBlocks = companionImageContentBlocks(m.Content, images)
			}
			msgs = append(msgs, msg)
			conversation = append(conversation, conversationMessage{Role: m.Role, Content: m.Content})
		}
	}

	rp := r.svc.src.Resolve()
	rp.WorkDir = r.svc.workDir
	client, err := r.svc.factory(rp)
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.active[runID] = cancel
	r.mu.Unlock()

	intent := resolveCompanionIntentWithConversation(text, requestIntent, conversation)
	if intent.IsReadOnly() && len(msgs) > 0 {
		msgs[0].Content += "\n" + readOnlyTurnInstruction(language)
	}
	if r.svc.history != nil {
		if err := r.svc.history.Append(ctx, HistoryMessage{
			ProjectID: projectID,
			NodeID:    nodeID,
			RunID:     runID,
			Role:      "user",
			Scope:     scope,
			Intent:    string(intent.Kind),
			Status:    HistoryStatusDone,
			Content:   text,
			CreatedAt: userAt,
		}); err != nil {
			r.finish(runID)
			return "", err
		}
	}
	go r.run(runCtx, runID, projectID, nodeID, scope, path, text, msgs, client, intent, language, now)
	return runID, nil
}

func turnHistoryScope(requestedScope, nodeID string) string {
	switch strings.TrimSpace(requestedScope) {
	case HistoryScopeProject:
		return HistoryScopeProject
	case HistoryScopeScene:
		return normalizeHistoryScope(HistoryScopeScene, nodeID)
	default:
		if strings.TrimSpace(nodeID) != "" {
			return HistoryScopeScene
		}
		return HistoryScopeProject
	}
}

func companionImageContentBlocks(text string, images []ImageAttachment) []llm.ContentBlock {
	blocks := make([]llm.ContentBlock, 0, len(images)+1)
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, llm.ContentBlock{Type: "text", Text: text})
	}
	for _, image := range images {
		blocks = append(blocks, llm.ContentBlock{
			Type:      "image",
			MediaType: image.MediaType,
			Data:      image.Data,
		})
	}
	return blocks
}

// watchForStall aborts a run that has gone completely silent. It marks the run
// as timed out before cancelling so the caller reports a retryable error rather
// than a user-requested stop.
func (r *Runner) watchForStall(ctx context.Context, runID string, lastActivity *atomic.Int64, timedOut *atomic.Bool, done <-chan struct{}) {
	ticker := time.NewTicker(companionStallCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(time.UnixMilli(lastActivity.Load())) < companionStallTimeout {
				continue
			}
			timedOut.Store(true)
			_ = r.cancel(runID)
			return
		}
	}
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

// A run with no deltas, tool calls, or turn ends for this long is treated as a
// dead connection rather than slow work: outline runs take minutes, but they
// keep producing something. Vars so tests can shorten them.
var (
	companionStallTimeout       = 5 * time.Minute
	companionStallCheckInterval = 15 * time.Second
)

const applyOpsToolName = "linetta_apply_ops"

func (r *Runner) run(ctx context.Context, runID, projectID, nodeID, scope, path, userText string, msgs []llm.ChatMessage, client llm.Client, intent companionIntent, language string, now func() int64) {
	defer r.finish(runID)

	intentName := string(intent.Kind)
	registry := r.svc.buildToolRegistryWithIntent(projectID, nodeID, scope, now, intent, language, runID, userText)
	notifyPhase := func(phase, text string, applied, total int) {
		_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{
			RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName,
			Text: text, Phase: phase, Applied: applied, Total: total,
		})
	}
	lastActivity := &atomic.Int64{}
	lastActivity.Store(time.Now().UnixMilli())
	touch := func() { lastActivity.Store(time.Now().UnixMilli()) }
	var timedOut atomic.Bool
	watchdogDone := make(chan struct{})
	defer close(watchdogDone)
	go r.watchForStall(ctx, runID, lastActivity, &timedOut, watchdogDone)
	generatingAnnounced := false
	previewPending := false
	dedup := streamdedup.New()
	queryRounds := 0
	forcedTool := companionForcedToolForIntent(intent)
	forcedApplyOps := forcedTool == applyOpsToolName
	applyOpsAttempted := false
	applyOpsSucceeded := false
	sceneTextSucceeded := false
	applyOpsFallbackApplied := false
	var lastApplyOpsResult ApplyOpsResult
	applyOpsCorrectionUsed := false
	client = newFirstTurnToolChoiceClient(client, forcedTool)
	loop := agentloop.New(client, registry, agentloop.HookFunc(func(ctx context.Context, evt agentloop.Event) {
		switch evt.Type {
		case agentloop.EventBeforeTool:
			touch()
			if evt.ToolName == applyOpsToolName {
				applyOpsAttempted = true
			}
			notifyPhase(toolPhase(evt.ToolName), friendlyToolLabel(evt.ToolName, language), 0, 0)
		case agentloop.EventAfterTool:
			touch()
			if evt.ToolName == applyOpsToolName && !evt.ToolIsError {
				applyOpsSucceeded = true
				applied := 0
				pending := false
				if result, ok := parseApplyOpsToolResult(evt.ToolResult); ok {
					lastApplyOpsResult = result
					applied = result.Applied
					pending = result.PendingApproval
					if intent.RequiresSceneText() && applyOpsResultHasSceneTextChange(result) {
						sceneTextSucceeded = true
					}
				}
				if pending {
					// The batch is waiting on the writer, so the run is done working.
					previewPending = true
					notifyPhase(phaseAwaitingApproval, awaitingApprovalStatusText(language), 0, 0)
					return
				}
				notifyPhase(phaseApplied, appliedStatusText(language), applied, applied)
			}
		}
	}))
	notifyPhase(phaseRequesting, requestingStatusText(language), 0, 0)
	resp, err := loop.Run(ctx, msgs, agentloop.RunOptions{
		MaxIterations: 8,
		Tools:         registry.Schemas(),
		OnDelta: func(text string) {
			touch()
			if !generatingAnnounced && strings.TrimSpace(text) != "" {
				generatingAnnounced = true
				notifyPhase(phaseGenerating, generatingStatusText(language), 0, 0)
			}
			switch act, payload := dedup.Observe(text); act {
			case streamdedup.ActionEmit:
				_ = r.svc.notify.Notify("companion.delta", deltaPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Text: payload})
			case streamdedup.ActionReset:
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Text: payload})
			case streamdedup.ActionSkip:
			}
		},
		OnReasoningDelta: func(text string) {
			touch()
			if strings.TrimSpace(text) == "" {
				return
			}
			_ = r.svc.notify.Notify("companion.reasoning", reasoningPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Text: text})
		},
		OnTurnEnd: func(ctx context.Context, lastResp llm.ChatResponse) (string, error) {
			touch()
			if forcedApplyOps && !applyOpsSucceeded && !applyOpsCorrectionUsed {
				applyOpsCorrectionUsed = true
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Text: ""})
				notifyPhase(phaseVerifying, friendlyToolLabel(applyOpsToolName, language), 0, 0)
				dedup = streamdedup.New()
				generatingAnnounced = false
				return directApplyCorrectionPrompt(userText, intent, language), nil
			}
			if queryRounds >= maxQueryRounds-1 {
				return "", nil
			}
			full := lastResp.Message.Content
			if qr, present, qerr := ParseQuery(full); present && qerr == nil {
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Text: ""})
				notifyPhase(phaseQuerying, querySummary(qr, language), 0, 0)
				queryRounds++
				dedup = streamdedup.New()
				generatingAnnounced = false
				return r.svc.runQueries(ctx, projectID, qr.Queries, language), nil
			}
			return "", nil
		},
	})
	if ctx.Err() != nil {
		if timedOut.Load() {
			msg := timedOutMessage(language)
			r.recordAssistantHistory(runID, projectID, nodeID, scope, intent, HistoryStatusFailed, msg, now())
			_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Message: msg})
			return
		}
		// An apply that already started runs to completion (see buildApplyOpsTool),
		// so a stop lands either before any change or after a whole one.
		msg := cancelledMessage(language)
		if applyOpsSucceeded {
			msg = cancelledAfterApplyMessage(language)
		}
		r.recordAssistantHistory(runID, projectID, nodeID, scope, intent, HistoryStatusCancelled, msg, now())
		_ = r.svc.notify.Notify("companion.cancelled", cancelledPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Applied: applyOpsSucceeded})
		return
	}
	if err != nil {
		r.recordAssistantHistory(runID, projectID, nodeID, scope, intent, HistoryStatusFailed, err.Error(), now())
		_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Message: err.Error()})
		return
	}
	full := resp.Message.Content
	if full == "" {
		full = dedup.Final()
	}
	if forcedApplyOps && !applyOpsSucceeded {
		if result, ok := r.applyDirectProposalFallback(ctx, runID, projectID, nodeID, scope, userText, full, intent, language, now); ok {
			applyOpsFallbackApplied = true
			applyOpsSucceeded = true
			lastApplyOpsResult = result
			if intent.RequiresSceneText() && applyOpsResultHasSceneTextChange(result) {
				sceneTextSucceeded = true
			}
		}
	}
	if applyOpsAttempted && !applyOpsSucceeded && strings.TrimSpace(full) == "" {
		msg := applyOpsFailedMessage(language)
		r.recordAssistantHistory(runID, projectID, nodeID, scope, intent, HistoryStatusFailed, msg, now())
		_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Message: msg})
		return
	}
	if intent.RequiresSceneText() && !sceneTextSucceeded && !previewPending {
		msg := sceneTextApplyFailedMessage(language)
		r.recordAssistantHistory(runID, projectID, nodeID, scope, intent, HistoryStatusFailed, msg, now())
		_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Message: msg})
		return
	}
	if intent.RequiresSceneText() && sceneTextSucceeded {
		full = sceneTextApplySuccessMessage(lastApplyOpsResult, language)
	} else if applyOpsFallbackApplied {
		full = applyOpsFallbackSuccessMessage(lastApplyOpsResult, language)
	}

	assistantAt := now()
	if err := session.AppendMessage(path, session.Message{Role: "assistant", Content: full, Timestamp: time.UnixMilli(assistantAt)}); err != nil {
		r.svc.recordPersistenceError(ctx, assistantAt, "assistant", path, err)
		_ = r.svc.notify.Notify("companion.error", errorPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Message: "companion transcript: " + err.Error()})
		return
	}
	r.svc.recordPersistenceOK(ctx, assistantAt, "assistant", path)
	status := HistoryStatusDone
	if intent.RequiresSceneText() && sceneTextSucceeded {
		status = HistoryStatusApplied
	}
	r.recordAssistantHistory(runID, projectID, nodeID, scope, intent, status, full, assistantAt)
	if prop, present, perr := ParseProposal(full); present {
		pp := proposalPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Valid: perr == nil, Summary: prop.Summary, Ops: prop.Ops}
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
			ProjectID:   projectID,
			NodeID:      nodeID,
			Scope:       scope,
			Intent:      intentName,
			Prompt:      ch.Prompt,
			Options:     ch.Options,
			AllowCustom: ch.AllowCustom,
		})
	}
	_ = r.svc.notify.Notify("companion.done", donePayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, FullText: full})
}

func (r *Runner) applyDirectProposalFallback(ctx context.Context, runID, projectID, nodeID, scope, userText, full string, intent companionIntent, language string, now func() int64) (ApplyOpsResult, bool) {
	prop, present, err := ParseProposal(full)
	if !present || err != nil {
		return ApplyOpsResult{}, false
	}
	if err := validateApplyOpsIntent(prop, companionApplyOpsIntent(userText, nodeID, intent)); err != nil {
		return ApplyOpsResult{}, false
	}
	intentName := string(intent.Kind)
	_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Text: friendlyToolLabel(applyOpsToolName, language)})
	result := r.svc.ApplyOps(ctx, projectID, nodeID, prop, now)
	if result.Applied > 0 {
		_ = r.svc.notify.Notify("companion.applied", appliedPayload{
			RunID:        runID,
			ProjectID:    projectID,
			NodeID:       nodeID,
			Scope:        scope,
			Intent:       intentName,
			Summary:      result.Summary,
			Applied:      result.Applied,
			ChangedNodes: result.ChangedNodes,
			UndoBatchID:  result.UndoBatchID,
		})
	}
	if result.Applied == 0 || result.isError() {
		return result, false
	}
	_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, ProjectID: projectID, NodeID: nodeID, Scope: scope, Intent: intentName, Text: appliedStatusText(language)})
	return result, true
}

func (r *Runner) recordAssistantHistory(runID, projectID, nodeID, scope string, intent companionIntent, status, content string, at int64) {
	if r.svc.history == nil {
		return
	}
	if strings.TrimSpace(content) == "" {
		return
	}
	_ = r.svc.history.Append(context.Background(), HistoryMessage{
		ProjectID: projectID,
		NodeID:    nodeID,
		RunID:     runID,
		Role:      "assistant",
		Scope:     scope,
		Intent:    string(intent.Kind),
		Status:    status,
		Content:   content,
		CreatedAt: at,
	})
}

// querySummary returns a short "조회 중: toolA, toolB" status string.
func querySummary(qr QueryRequest, lang string) string {
	names := make([]string, 0, len(qr.Queries))
	for _, q := range qr.Queries {
		names = append(names, q.Tool)
	}
	if isEnglish(lang) {
		return "Looking up: " + strings.Join(names, ", ")
	}
	if isJapanese(lang) {
		return "照会中: " + strings.Join(names, ", ")
	}
	return "조회 중: " + strings.Join(names, ", ")
}

func companionForcedToolForUserText(text string) string {
	return companionForcedToolForIntent(classifyCompanionIntent(text))
}

func companionForcedToolForIntent(intent companionIntent) string {
	if intent.RequiresApplyOps() {
		return applyOpsToolName
	}
	return ""
}

func parseApplyOpsToolResult(raw string) (ApplyOpsResult, bool) {
	var result ApplyOpsResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return ApplyOpsResult{}, false
	}
	return result, true
}

func applyOpsResultHasSceneTextChange(result ApplyOpsResult) bool {
	for _, change := range result.ChangedNodes {
		if change.Op == "set_scene_text" && change.NodeID != "" && change.ContentVersion > 0 {
			return true
		}
	}
	return false
}

func sceneTextApplySuccessMessage(result ApplyOpsResult, lang string) string {
	if isJapanese(lang) {
		if len(result.ChangedNodes) == 0 {
			return "現在のシーン本文を反映しました。"
		}
		change := result.ChangedNodes[0]
		var b strings.Builder
		if change.CharCount > 0 {
			fmt.Fprintf(&b, "現在のシーン本文を反映しました。（%d字）", change.CharCount)
		} else {
			b.WriteString("現在のシーン本文を反映しました。")
		}
		b.WriteString("\n\n作業の流れ\n")
		if strings.TrimSpace(result.Summary) != "" {
			b.WriteString("- リクエスト処理: ")
			b.WriteString(strings.TrimSpace(result.Summary))
			b.WriteString("\n")
		}
		b.WriteString("- 現在のシーンの文脈とご指示に基づいて本文を執筆しました。\n")
		b.WriteString("- 執筆した原稿を現在のシーン本文として直接適用しました。")
		if preview := sceneTextPreviewForMessage(change.TextPreview); preview != "" {
			b.WriteString("\n- 冒頭: \"")
			b.WriteString(preview)
			b.WriteString("\"")
		}
		return b.String()
	}
	if isEnglish(lang) {
		if len(result.ChangedNodes) == 0 {
			return "Applied the current scene text."
		}
		change := result.ChangedNodes[0]
		var b strings.Builder
		if change.CharCount > 0 {
			fmt.Fprintf(&b, "Applied the current scene text. (%d chars)", change.CharCount)
		} else {
			b.WriteString("Applied the current scene text.")
		}
		b.WriteString("\n\nWork log\n")
		if strings.TrimSpace(result.Summary) != "" {
			b.WriteString("- Request: ")
			b.WriteString(strings.TrimSpace(result.Summary))
			b.WriteString("\n")
		}
		b.WriteString("- Wrote the prose from the current scene's context and your instructions.\n")
		b.WriteString("- Applied the draft directly as the current scene text.")
		if preview := sceneTextPreviewForMessage(change.TextPreview); preview != "" {
			b.WriteString("\n- Opening: \"")
			b.WriteString(preview)
			b.WriteString("\"")
		}
		return b.String()
	}
	if len(result.ChangedNodes) == 0 {
		return "현재 씬 본문을 반영했습니다."
	}
	change := result.ChangedNodes[0]
	var b strings.Builder
	if change.CharCount > 0 {
		fmt.Fprintf(&b, "현재 씬 본문을 반영했습니다. (%d자)", change.CharCount)
	} else {
		b.WriteString("현재 씬 본문을 반영했습니다.")
	}
	b.WriteString("\n\n작업 흐름\n")
	if strings.TrimSpace(result.Summary) != "" {
		b.WriteString("- 요청 처리: ")
		b.WriteString(strings.TrimSpace(result.Summary))
		b.WriteString("\n")
	}
	b.WriteString("- 현재 씬의 맥락과 사용자 지시를 바탕으로 본문을 작성했습니다.\n")
	b.WriteString("- 작성한 원고를 현재 씬 본문으로 바로 적용했습니다.")
	if preview := sceneTextPreviewForMessage(change.TextPreview); preview != "" {
		b.WriteString("\n- 시작 부분: \"")
		b.WriteString(preview)
		b.WriteString("\"")
	}
	return b.String()
}

func applyOpsFallbackSuccessMessage(result ApplyOpsResult, lang string) string {
	count := result.Applied
	if isJapanese(lang) {
		if count <= 0 {
			return "作品の状態を反映しました。"
		}
		var b strings.Builder
		if count == 1 {
			b.WriteString("作品の状態を反映しました。")
		} else {
			fmt.Fprintf(&b, "作品の状態に %d 件の変更を反映しました。", count)
		}
		if strings.TrimSpace(result.Summary) != "" {
			b.WriteString("\n\n作業の流れ\n- リクエスト処理: ")
			b.WriteString(strings.TrimSpace(result.Summary))
		}
		return b.String()
	}
	if isEnglish(lang) {
		if count <= 0 {
			return "Applied the story state changes."
		}
		var b strings.Builder
		if count == 1 {
			b.WriteString("Applied the story state changes.")
		} else {
			fmt.Fprintf(&b, "Applied %d changes to the story state.", count)
		}
		if strings.TrimSpace(result.Summary) != "" {
			b.WriteString("\n\nWork log\n- Request: ")
			b.WriteString(strings.TrimSpace(result.Summary))
		}
		return b.String()
	}
	if count <= 0 {
		return "작품 상태를 반영했습니다."
	}
	var b strings.Builder
	if count == 1 {
		b.WriteString("작품 상태를 반영했습니다.")
	} else {
		fmt.Fprintf(&b, "작품 상태에 %d개 변경을 반영했습니다.", count)
	}
	if strings.TrimSpace(result.Summary) != "" {
		b.WriteString("\n\n작업 흐름\n- 요청 처리: ")
		b.WriteString(strings.TrimSpace(result.Summary))
	}
	return b.String()
}

func sceneTextPreviewForMessage(preview string) string {
	preview = strings.Join(strings.Fields(preview), " ")
	return trimRunesLocal(preview, 80)
}

func sceneTextApplyFailedMessage(lang string) string {
	if isEnglish(lang) {
		return "No text change was produced. Try again or check the current scene."
	}
	if isJapanese(lang) {
		return "本文の変更が作成されませんでした。再試行するか、現在のシーンを確認してください。"
	}
	return "본문 변경이 만들어지지 않았습니다. 다시 시도하거나 현재 씬을 확인해주세요."
}

func applyOpsFailedMessage(lang string) string {
	if isEnglish(lang) {
		return "Could not apply the story changes. Please try again."
	}
	if isJapanese(lang) {
		return "作品の変更を適用できませんでした。もう一度お試しください。"
	}
	return "작품 변경을 적용하지 못했습니다. 다시 시도해주세요."
}

func directApplyCorrectionPrompt(userText string, intent companionIntent, lang string) string {
	userText = strings.TrimSpace(userText)
	if isJapanese(lang) {
		if userText == "" {
			userText = "（原文なし）"
		}
		if intent.RequiresSceneText() {
			return "直前のユーザーリクエストは説明や提案ではなく、実際の作品状態の変更リクエストです。" +
				"現在のシーン本文の執筆/修正/確定を求めているため、追加の質問や選択肢で終わらせないでください。" +
				"すでに提供されている現在のシーン、前後の流れ、ユーザーの指示に基づいてすぐ読めるシーン本文を書き、" +
				"必ず linetta_apply_ops の set_scene_text で実際のシーン原稿を置き換えてください。" +
				"変更したと言葉だけで答えず、ツールを呼び出してください。\n\n" +
				"ユーザーリクエスト: " + userText
		}
		return "直前のユーザーリクエストは説明や提案ではなく、実際の作品状態の変更リクエストです。" +
			"変更したと言葉だけで答えず、linetta_apply_ops を呼び出して作品状態に適用してください。" +
			"現在のシーン本文の書き直し/修正/拡張/推敲のリクエストなら set_scene_text で実際のシーン原稿を置き換えてください。" +
			"アウトライン/目次のリクエストなら create_outline_node または create_scene で左のアウトラインツリーを作り、必要に応じてそのノードに create_thread/add_beat を繋げてください。" +
			"現在の情報だけで適用できない場合は適用せず、足りない情報を一文で質問してください。\n\n" +
			"ユーザーリクエスト: " + userText
	}
	if isEnglish(lang) {
		if userText == "" {
			userText = "(no original text)"
		}
		if intent.RequiresSceneText() {
			return "The user's last request is an actual story-state change request, not a question or proposal. " +
				"It asks you to write/revise/finalize the current scene text, so do not end with more questions or options. " +
				"Write readable scene prose from the current scene, surrounding flow, and the user's instructions, " +
				"and you MUST replace the actual scene text via linetta_apply_ops set_scene_text. " +
				"Do not merely claim you changed it — call the tool.\n\n" +
				"User request: " + userText
		}
		return "The user's last request is an actual story-state change request, not a question or proposal. " +
			"Do not merely claim you changed it — call linetta_apply_ops to apply it to the story state. " +
			"If it asks to rewrite/revise/expand/polish the current scene text, replace the actual prose via set_scene_text. " +
			"If it is an outline/table-of-contents request, build the left outline tree with create_outline_node or create_scene, attaching create_thread/add_beat to those nodes as needed. " +
			"If you cannot apply it with the information at hand, do not apply — ask one sentence for the missing detail.\n\n" +
			"User request: " + userText
	}
	if userText == "" {
		userText = "(원문 없음)"
	}
	if intent.RequiresSceneText() {
		return "방금 사용자 요청은 설명이나 제안이 아니라 실제 작품 상태 변경 요청입니다. " +
			"현재 씬 본문을 작성/수정/확정해 달라는 요청이므로 추가 질문이나 선택지로 끝내지 마세요. " +
			"이미 제공된 현재 씬, 앞뒤 흐름, 사용자 지시를 바탕으로 바로 읽을 수 있는 씬 본문을 쓰고, " +
			"반드시 linetta_apply_ops의 set_scene_text로 실제 씬 원고를 교체하세요. " +
			"변경했다고 말로만 답하지 말고 도구를 호출하세요.\n\n" +
			"사용자 요청: " + userText
	}
	return "방금 사용자 요청은 설명이나 제안이 아니라 실제 작품 상태 변경 요청입니다. " +
		"변경했다고 말로만 답하지 말고 linetta_apply_ops를 호출해 작품 상태에 적용하세요. " +
		"현재 씬의 본문 재작성/수정/확장/다듬기 요청이면 set_scene_text로 실제 씬 원고를 교체하세요. " +
		"아웃라인/목차 요청이면 create_outline_node 또는 create_scene으로 왼쪽 아웃라인 트리를 만들고, 필요한 경우 그 노드에 create_thread/add_beat를 함께 연결하세요. " +
		"현재 정보만으로 적용할 수 없으면 적용하지 말고 부족한 정보를 한 문장으로 질문하세요.\n\n" +
		"사용자 요청: " + userText
}

var companionStructureTerms = []string{
	"스토리라인", "줄거리", "플롯", "비트", "캐릭터", "인물", "관계",
	"장소", "씬", "장면", "개요", "요약", "기억", "설정", "세계관",
	"시놉시스", "아웃라인", "얼개", "구조", "챕터", "막", "파트",
	"에피소드", "회차", "본문", "원고", "문장",
	// English equivalents (input is lower-cased before matching).
	"storyline", "plot", "beat", "character", "relationship", "place",
	"scene", "overview", "synopsis", "outline", "structure", "chapter",
	"episode", "manuscript", "prose", "draft", "worldbuilding", "lore",
	// Japanese equivalents.
	"ストーリーライン", "プロット", "ビート", "キャラクター", "人物",
	"関係", "場所", "シーン", "場面", "概要", "あらすじ", "記憶", "設定",
	"世界観", "シノプシス", "アウトライン", "構成", "章", "パート",
	"エピソード", "本文", "原稿", "文章",
}

var companionMutationTerms = []string{
	"수정", "추가", "생성", "만들", "바꿔", "변경", "반영", "저장",
	"붙여", "넣어", "정리", "업데이트", "삭제", "지워", "작성",
	"써", "짜", "구성", "잡아", "세워", "완성", "나눠", "나누",
	"쪼개", "분할", "세분", "구체화", "확장", "전개", "다듬",
	"고쳐", "재작성", "초기화", "비워", "채워",
	// English equivalents.
	"write", "add", "create", "make", "change", "update", "save",
	"delete", "remove", "revise", "rewrite", "expand", "split",
	"refine", "organize", "polish", "fill in", "clear", "edit",
	// Japanese equivalents.
	"修正", "追加", "作成", "作って", "変えて", "変更", "反映", "保存",
	"入れて", "整理", "更新", "削除", "消して", "執筆", "書いて",
	"完成", "分けて", "分割", "具体化", "拡張", "展開", "整えて",
	"直して", "書き直", "初期化", "埋めて",
}

var companionEducationalTerms = []string{
	"작성법", "방법", "어떻게", "가이드",
	// English equivalents.
	"how to", "how do", "how can", "guide", "method",
	// Japanese equivalents.
	"書き方", "方法", "どうやって", "ガイド",
}

var companionResearchTerms = []string{
	"검색", "찾아", "조사", "웹", "web", "url", "링크", "자료", "최신",
	"레퍼런스",
	// English equivalents.
	"search", "find", "look up", "research", "link", "reference",
	"latest", "source",
	// Japanese equivalents.
	"検索", "調べて", "調査", "リンク", "資料", "最新", "リファレンス",
}

var companionDirectApplyTerms = []string{
	"해줘", "해 줘", "해주세요", "해 주세요", "반영해", "저장해", "수정해",
	"추가해", "만들어", "넣어", "붙여", "작성해", "써줘", "써 줘",
	"짜줘", "짜 줘", "구성해", "잡아줘", "잡아 줘", "세워줘", "세워 줘",
	"나눠줘", "나눠 줘", "쪼개줘", "쪼개 줘", "구체화해", "확장해",
	"전개해", "다듬어", "고쳐줘", "고쳐 줘", "재작성해",
	// English direct-request markers.
	"please", "go ahead", "do it", "apply it", "write it", "make it",
	"add it", "save it", "update it", "just write", "just apply",
	// Japanese direct-request markers.
	"してください", "してくれ", "お願いします", "反映して", "保存して",
	"追加して", "書いて", "作成して", "適用して", "作って",
}

var companionDiscussionTerms = []string{
	"어때", "추천", "아이디어", "설명", "알려", "검토", "브레인스토밍",
	"가능할까", "괜찮을까",
	// English equivalents.
	"what do you think", "recommend", "suggestion", "idea", "explain",
	"tell me", "review", "brainstorm", "what if", "thoughts",
	// Japanese equivalents.
	"どう思う", "おすすめ", "アイデア", "説明", "教えて", "レビュー",
	"ブレインストーミング", "可能かな", "大丈夫かな",
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
