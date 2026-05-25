package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/google/uuid"
)

// Clock matches handlers.Clock.
type Clock func() int64

// Runner manages active AI runs.
type Runner struct {
	notify   rpc.Notifier
	runs     *store.AIRunsRepo
	factory  ClientFactory
	provider string
	workDir  string

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

// NewRunner constructs a Runner. workDir is passed to claude-code-cli; can be
// the empty string (current working dir).
func NewRunner(notify rpc.Notifier, runs *store.AIRunsRepo, factory ClientFactory, provider string) *Runner {
	return &Runner{
		notify:   notify,
		runs:     runs,
		factory:  factory,
		provider: provider,
		active:   map[string]context.CancelFunc{},
	}
}

// Start enqueues a run and returns its id immediately. The work happens on a
// goroutine that emits notifications via the Notifier.
func (r *Runner) Start(ctx context.Context, c Context, now Clock) (string, error) {
	runID := uuid.NewString()
	startedAt := now()
	ctxJSON, _ := json.Marshal(c)

	var nodeID *string
	if c.NodeID != "" {
		v := c.NodeID
		nodeID = &v
	}
	if err := r.runs.Insert(ctx, store.AIRun{
		ID: runID, ProjectID: c.ProjectID, NodeID: nodeID,
		Provider:    r.provider,
		Prompt:      c.UserPrompt,
		ContextJSON: ctxJSON,
		Status:      store.AIRunStreaming,
		StartedAt:   startedAt,
	}); err != nil {
		return "", err
	}

	client, err := r.factory(r.provider, r.workDir)
	if err != nil {
		_ = r.runs.UpdateStatus(ctx, runID, store.AIRunError, "", err.Error(), now())
		return "", err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.active[runID] = cancel
	r.mu.Unlock()

	go r.run(runCtx, runID, c, client, now)
	return runID, nil
}

func (r *Runner) run(ctx context.Context, runID string, c Context, client llm.Client, now Clock) {
	defer func() {
		r.mu.Lock()
		delete(r.active, runID)
		r.mu.Unlock()
	}()

	var full struct {
		mu  sync.Mutex
		buf string
	}
	msgs := BuildMessages(c)

	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{
		OnDelta: func(text string) {
			full.mu.Lock()
			full.buf += text
			full.mu.Unlock()
			_ = r.notify.Notify("ai.delta", DeltaPayload{RunID: runID, Text: text})
		},
	})

	endedAt := now()
	if errors.Is(err, context.Canceled) {
		_ = r.notify.Notify("ai.cancelled", CancelledPayload{RunID: runID})
		full.mu.Lock()
		out := full.buf
		full.mu.Unlock()
		_ = r.runs.UpdateStatus(context.Background(), runID, store.AIRunCancelled, out, "", endedAt)
		return
	}
	if err != nil {
		_ = r.notify.Notify("ai.error", ErrorPayload{RunID: runID, Message: err.Error()})
		full.mu.Lock()
		out := full.buf
		full.mu.Unlock()
		_ = r.runs.UpdateStatus(context.Background(), runID, store.AIRunError, out, err.Error(), endedAt)
		return
	}

	finalText := resp.Message.Content
	if finalText == "" {
		full.mu.Lock()
		finalText = full.buf
		full.mu.Unlock()
	}
	_ = r.notify.Notify("ai.done", DonePayload{RunID: runID, FullText: finalText})
	_ = r.runs.UpdateStatus(context.Background(), runID, store.AIRunDone, finalText, "", endedAt)
}

// Cancel cancels the run by id. Returns an error if no such run is active.
func (r *Runner) Cancel(runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.active[runID]
	if !ok {
		return errors.New("ai: run not found or already finished")
	}
	cancel()
	return nil
}
