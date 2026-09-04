//go:build !mobile

package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

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
}

// New wires the service. Nothing connects until the first run.
func New(d Deps) *Service {
	return &Service{
		deps: d,
		tr:   &transcript{repo: d.History, clock: d.Clock},
		runs: newRunRegistry(),
	}
}

// Close tears down the tool session.
func (s *Service) Close() error {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()
	if s.tools == nil {
		return nil
	}
	err := s.tools.Close()
	s.tools = nil
	return err
}

// session returns the connected tool session, building it once. A failed
// attempt leaves nothing cached, so the next run retries instead of
// inheriting a broken session forever.
func (s *Service) session(ctx context.Context) (*toolSession, error) {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()
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
