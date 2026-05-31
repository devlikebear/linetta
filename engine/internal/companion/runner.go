package companion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/streamdedup"
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
type thinkingPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
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

	client, err := r.svc.factory(r.svc.src.Provider(), r.svc.workDir)
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runID := uuid.NewString()
	r.mu.Lock()
	r.active[runID] = cancel
	r.mu.Unlock()

	go r.run(runCtx, runID, projectID, path, msgs, client, now)
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

func (r *Runner) run(ctx context.Context, runID, projectID, path string, msgs []llm.ChatMessage, client llm.Client, now func() int64) {
	defer r.finish(runID)

	for round := 0; round < maxQueryRounds; round++ {
		dedup := streamdedup.New()
		resp, err := client.Chat(ctx, msgs, llm.ChatOptions{
			OnDelta: func(text string) {
				switch act, payload := dedup.Observe(text); act {
				case streamdedup.ActionEmit:
					_ = r.svc.notify.Notify("companion.delta", deltaPayload{RunID: runID, Text: payload})
				case streamdedup.ActionReset:
					_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, Text: payload})
				case streamdedup.ActionSkip:
				}
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

		// If not the last allowed round, check for a read-query and loop.
		if round < maxQueryRounds-1 {
			if qr, present, qerr := ParseQuery(full); present && qerr == nil {
				// This round was a query, not the final answer: clear partial prose,
				// surface a thinking status, run reads, feed results, continue.
				_ = r.svc.notify.Notify("companion.reset", resetPayload{RunID: runID, Text: ""})
				_ = r.svc.notify.Notify("companion.thinking", thinkingPayload{RunID: runID, Text: querySummary(qr)})
				result := r.svc.runQueries(ctx, projectID, qr.Queries)
				msgs = append(msgs,
					llm.ChatMessage{Role: "assistant", Content: full},
					llm.ChatMessage{Role: "user", Content: result},
				)
				continue
			}
		}

		// Final round.
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
		_ = r.svc.notify.Notify("companion.done", donePayload{RunID: runID, FullText: full})
		return
	}
}

// querySummary returns a short "조회 중: toolA, toolB" status string.
func querySummary(qr QueryRequest) string {
	names := make([]string, 0, len(qr.Queries))
	for _, q := range qr.Queries {
		names = append(names, q.Tool)
	}
	return "조회 중: " + strings.Join(names, ", ")
}
