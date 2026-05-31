package companion

import (
	"context"
	"path/filepath"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/thread"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/session"
)

// historyTokenBudget caps how much prior transcript is replayed into context.
const historyTokenBudget = 6000

// entityContextLimit caps how many entities are injected.
const entityContextLimit = 40

// ClientFactory mirrors ai.ClientFactory (provider id + workDir -> llm.Client).
type ClientFactory func(provider, workDir string) (llm.Client, error)

// ProviderSource yields the current provider id (settings.Store satisfies it).
type ProviderSource interface{ Provider() string }

// Service wires the companion backend.
type Service struct {
	sessions      *session.Store
	projects      *project.Repo
	threads       *thread.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	plot          *plot.Builder
	nodes         *node.Repo
	beats         *beat.Repo
	notify        rpc.Notifier
	factory       ClientFactory
	src           ProviderSource
	workDir       string
	runner        *Runner
	memBase       string
}

// NewService constructs the companion service. sessionsDir is passed to
// session.NewStore (e.g. <home>/companion).
func NewService(
	sessionsDir string,
	projects *project.Repo, threads *thread.Repo, entities *entity.Repo,
	relationships *relationship.Repo, plotBuilder *plot.Builder,
	notify rpc.Notifier, factory ClientFactory, src ProviderSource, workDir string,
	nodes *node.Repo, beats *beat.Repo,
) *Service {
	s := &Service{
		sessions:      session.NewStore(sessionsDir),
		projects:      projects, threads: threads, entities: entities,
		relationships: relationships, plot: plotBuilder,
		nodes: nodes, beats: beats,
		notify: notify, factory: factory, src: src, workDir: workDir,
		memBase: filepath.Join(sessionsDir, "mem"),
	}
	s.runner = newRunner(s)
	return s
}

// gatherContext loads project state for prompt injection. nodeID may be "".
func (s *Service) gatherContext(ctx context.Context, projectID, nodeID, query string) (PromptData, error) {
	proj, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return PromptData{}, err
	}
	d := PromptData{Outline: proj.Outline}

	resolvedNode := nodeID
	if resolvedNode == "" && proj.LastOpenedNodeID != nil {
		resolvedNode = *proj.LastOpenedNodeID
	}
	// Context fields below are best-effort: partial context is preferred over
	// aborting the turn, so per-section load errors are intentionally ignored.
	if resolvedNode != "" {
		if sp, err := s.plot.Build(ctx, resolvedNode); err == nil {
			d.Spine = sp
			d.HasSpine = true
		}
	}
	if ths, err := s.threads.ListByProject(ctx, projectID, false); err == nil {
		d.Threads = ths
	}
	if ents, err := s.entities.Search(ctx, projectID, "", entityContextLimit); err == nil {
		d.Entities = ents
	}
	if rels, err := s.relationships.ListByProject(ctx, projectID); err == nil {
		d.Relationships = rels
	}
	// Keyword memory can't do topical matching (SearchExperiences matches
	// summary-contains-query), so surface the most recent facts every turn
	// rather than substring-matching the full user message. `query` is kept
	// for a future smarter (e.g. semantic) recall.
	_ = query
	d.Memories = s.Recall(projectID, "", recallLimit)
	return d, nil
}

// History returns the project's companion transcript messages.
func (s *Service) History(ctx context.Context, projectID string) ([]session.Message, error) {
	sess, err := s.sessions.EnsureWorker(projectID)
	if err != nil {
		return nil, err
	}
	return session.ReadMessages(s.sessions.TranscriptPath(sess.ID))
}

// Send starts a companion turn; returns the run id. Streaming + proposal arrive
// via notifications.
func (s *Service) Send(ctx context.Context, projectID, nodeID, text string, now func() int64) (string, error) {
	return s.runner.start(ctx, projectID, nodeID, text, now)
}

// Cancel cancels an in-flight run.
func (s *Service) Cancel(runID string) error { return s.runner.cancel(runID) }
