//go:build !mobile

package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/tars/pkg/llm"
)

// ProviderSource resolves the writer's settings into a client. Read per turn,
// never cached: changing the model in settings applies to the next message
// without restarting the engine. *provider.Source satisfies this.
type ProviderSource interface {
	Client(id string) (llm.Client, provider.Resolved, error)
}

// MemorySource reads the curated memories for one turn. An interface rather
// than the repo so the prompt can be tested without a database, matching
// ScopeLookup.
type MemorySource interface {
	Memories(ctx context.Context, projectID string) (writerProfile, workNotes agentmemory.Document)
}

// SkillSource reads the skills available for one turn — the writer's
// (global) and this work's — already reduced to what may reach a prompt.
// The full contract an implementation owes, because prompt.go re-checks none
// of it and only orders, caps and formats what it is handed:
//
//   - Enabled only. A skill the writer switched off must not be listed.
//   - Guard-passed. Every returned Name and Description has been through
//     agentskills.Guard, so an invisible character cannot ride into the
//     system prompt inside a description. agentskills.Store.List already
//     does this — it reports a guard failure as a Diagnostic instead of
//     returning the skill — which is why engineapp's agentSkillSource
//     satisfies this clause by construction rather than by re-guarding.
//   - Body left off. The list is cheap precisely because bodies stay on
//     disk until linetta_read_skill fetches one; a Body here would put the
//     whole point of progressive disclosure back in the prompt.
//
// That reduction is the caller's job, not systemPrompt's — the same division
// MemorySource draws around the two curated documents. An interface rather
// than *agentskills.Store directly, matching MemorySource and ScopeLookup,
// so the prompt can be tested without a filesystem.
type SkillSource interface {
	Skills(ctx context.Context, projectID string) []agentskills.Skill
}

// Deps are the collaborators the agent needs.
type Deps struct {
	Providers ProviderSource
	History   *companion.HistoryRepo
	Scope     ScopeLookup
	// Register installs the full tool set on the agent's own server. The
	// caller binds it to an mcphost.ToolDeps carrying Source: SourceAgent,
	// its own limiter and its own storyops service.
	Register RegisterTools
	Notify   func(method string, params any)
	Language func() string
	// Memory supplies the two curated documents pasted into the system prompt.
	Memory MemorySource
	// Skills supplies the name-and-description list pasted into the system
	// prompt after the memory block. A nil Skills is valid — it renders as
	// no skills, the same way a nil Memory renders as both documents empty —
	// so a build that never wires one (or a test that constructs Deps
	// without it) still works.
	Skills SkillSource
	// SelfReviewEnabled reports whether the self-improvement pass may run
	// after a working turn (settings.agent_self_review_enabled, default on).
	// A func rather than a bool because it is read at the END of every turn,
	// not captured when the service is built: a writer who switches it off
	// mid-session must not get one more review out of the turn already in
	// flight. A nil func means enabled — the same "unwired collaborator
	// degrades to the documented default" rule Memory and Skills follow, and
	// the default here is on.
	SelfReviewEnabled func() bool
	// Undo reverts a structural batch. It must be bound to the SAME storyops
	// service the agent's tools use — undo batches live in memory on the
	// service, so any other instance simply does not have the batch.
	Undo  func(ctx context.Context, batchID string) error
	Clock func() int64
}

// Service is the built-in agent. One per engine.
type Service struct {
	deps Deps
	tr   *transcript
	runs *runRegistry

	// The tool session is built on the first run rather than at start-up:
	// a writer who never opens the panel should not pay for a second MCP
	// server, and Open must not fail because of one.
	toolsMu sync.Mutex
	tools   *toolSession
	closed  bool

	// wg tracks turns in flight so Close can wait for them to actually stop.
	// Cancelling a turn's context is not enough on its own: a turn that is
	// mid Chat-call or mid transcript-write needs to observe the
	// cancellation and unwind before it is safe to close the tool session
	// or let the caller close the store underneath a turn that outlived it.
	wg sync.WaitGroup
}

// New wires the service. Nothing connects until the first run.
func New(d Deps) *Service {
	return &Service{
		deps: d,
		tr:   &transcript{repo: d.History, clock: d.Clock},
		runs: newRunRegistry(),
	}
}

// Close tears down the tool session. It cancels every turn still running,
// waits for them to actually unwind, and refuses any further Run or session
// rebuild from then on — a turn that outlived Close would otherwise keep
// calling the provider and writing transcript rows against a store the
// caller is free to close the moment Close returns.
func (s *Service) Close() error {
	s.toolsMu.Lock()
	if s.closed {
		s.toolsMu.Unlock()
		return nil
	}
	s.closed = true
	tools := s.tools
	s.tools = nil
	s.toolsMu.Unlock()

	s.runs.cancelAll()
	s.wg.Wait()

	if tools == nil {
		return nil
	}
	return tools.Close()
}

// session returns the connected tool session, building it once. A failed
// attempt leaves nothing cached, so the next run retries instead of
// inheriting a broken session forever. Once the service is closed it always
// refuses, rather than silently rebuilding a session Close just tore down.
func (s *Service) session(ctx context.Context) (*toolSession, error) {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()
	if s.closed {
		return nil, errors.New("agent: service is closed")
	}
	if s.tools != nil {
		return s.tools, nil
	}
	ts, err := connectTools(ctx, s.deps.Register)
	if err != nil {
		return nil, err
	}
	s.tools = ts
	return ts, nil
}

// enter registers one turn with Close's wait group, under the same lock
// session uses for its closed check. A Close racing a Run either finishes
// first — in which case Run fails at session, above, and never calls enter —
// or is guaranteed to wait for the turn enter just admitted.
func (s *Service) enter() error {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()
	if s.closed {
		return errors.New("agent: service is closed")
	}
	s.wg.Add(1)
	return nil
}

// leave releases one turn registered with enter.
func (s *Service) leave() { s.wg.Done() }

func (s *Service) notify(method string, params any) {
	if s.deps.Notify != nil {
		s.deps.Notify(method, params)
	}
}

func (s *Service) language() string {
	if s.deps.Language == nil {
		return "en"
	}
	if lang := s.deps.Language(); lang != "" {
		return lang
	}
	return "en"
}

// History returns the panel's conversation for a work.
func (s *Service) History(ctx context.Context, projectID string, limit int) ([]companion.HistoryMessage, error) {
	return s.tr.load(ctx, projectID, limit)
}

// Clear drops the conversation. The activity log survives on purpose.
func (s *Service) Clear(ctx context.Context, projectID string) error {
	return s.tr.clear(ctx, projectID)
}

// Undo reverts a structural batch the agent applied.
func (s *Service) Undo(ctx context.Context, batchID string) error {
	if s.deps.Undo == nil {
		return errors.New("agent: undo is not wired")
	}
	return s.deps.Undo(ctx, batchID)
}

// Cancel stops a run. An unknown run id is not an error: the writer's stop
// click can land after the last token.
func (s *Service) Cancel(runID string) error {
	s.runs.cancel(runID)
	return nil
}

func newRunID() string { return uuid.NewString() }
