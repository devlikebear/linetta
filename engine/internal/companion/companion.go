package companion

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/thread"
	"github.com/devlikebear/tars/pkg/session"
)

// historyTokenBudget caps how much prior transcript is replayed into context.
const historyTokenBudget = 6000

// entityContextLimit caps how many entities are injected. Core story roles are
// prioritized within this cap so 주인공/빌런/메인무대 settings survive recency.
const entityContextLimit = 40

const compactHistoryMaxMessages = 24
const compactHistorySnippetRunes = 240

// ClientFactory and ProviderSource are shared with the ai package so the same
// settings adapter and default factory serve AI runs, companion, and summaries.
type ClientFactory = ai.ClientFactory
type ProviderSource = ai.ProviderSource

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
	ops           *opsstatus.Repo
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
		sessions: session.NewStore(sessionsDir),
		projects: projects, threads: threads, entities: entities,
		relationships: relationships, plot: plotBuilder,
		nodes: nodes, beats: beats,
		notify: notify, factory: factory, src: src, workDir: workDir,
		memBase: filepath.Join(sessionsDir, "mem"),
	}
	s.runner = newRunner(s)
	return s
}

func (s *Service) WithOpsStatus(repo *opsstatus.Repo) *Service {
	s.ops = repo
	return s
}

func (s *Service) recordPersistenceOK(ctx context.Context, at int64, phase string, path string) {
	if s.ops == nil {
		return
	}
	_ = s.ops.Record(ctx, opsstatus.JobCompanionPersistence, at, at, true, "", map[string]any{
		"phase": phase,
		"path":  path,
	})
}

func (s *Service) recordPersistenceError(ctx context.Context, at int64, phase string, path string, err error) {
	if s.ops == nil || err == nil {
		return
	}
	_ = s.ops.Record(ctx, opsstatus.JobCompanionPersistence, at, at, false, err.Error(), map[string]any{
		"phase": phase,
		"path":  path,
	})
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
		if core, coreErr := s.entities.ListCoreByProject(ctx, projectID, entityContextLimit); coreErr == nil {
			d.Entities = mergeCoreEntities(core, ents, entityContextLimit)
		} else {
			d.Entities = ents
		}
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

func mergeCoreEntities(core, recent []entity.Entity, limit int) []entity.Entity {
	if limit <= 0 {
		limit = len(core) + len(recent)
	}
	seen := make(map[string]bool, len(core)+len(recent))
	capacity := len(core) + len(recent)
	if capacity > limit {
		capacity = limit
	}
	out := make([]entity.Entity, 0, capacity)
	add := func(list []entity.Entity) {
		for _, e := range list {
			if e.ID == "" || seen[e.ID] || len(out) >= limit {
				continue
			}
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	add(core)
	add(recent)
	return out
}

// History returns the project's companion transcript messages.
func (s *Service) History(ctx context.Context, projectID string) ([]session.Message, error) {
	sess, err := s.sessions.EnsureWorker(projectID)
	if err != nil {
		return nil, err
	}
	return session.ReadMessages(s.sessions.TranscriptPath(sess.ID))
}

// CompactHistory replaces a long companion transcript with one assistant
// summary message so future turns keep the useful context without replaying
// every prior exchange.
func (s *Service) CompactHistory(ctx context.Context, projectID string, now func() int64) ([]session.Message, error) {
	sess, err := s.sessions.EnsureWorker(projectID)
	if err != nil {
		return nil, err
	}
	path := s.sessions.TranscriptPath(sess.ID)
	msgs, err := session.ReadMessages(path)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	at := time.Now().UTC()
	if now != nil {
		at = time.UnixMilli(now()).UTC()
	}
	compacted := []session.Message{{
		Role:      "assistant",
		Content:   compactTranscriptSummary(msgs),
		Timestamp: at,
	}}
	if err := session.RewriteMessages(path, compacted); err != nil {
		return nil, err
	}
	return compacted, nil
}

// ClearHistory removes all persisted companion transcript messages.
func (s *Service) ClearHistory(ctx context.Context, projectID string) error {
	sess, err := s.sessions.EnsureWorker(projectID)
	if err != nil {
		return err
	}
	return session.RewriteMessages(s.sessions.TranscriptPath(sess.ID), nil)
}

func compactTranscriptSummary(msgs []session.Message) string {
	start := 0
	if len(msgs) > compactHistoryMaxMessages {
		start = len(msgs) - compactHistoryMaxMessages
	}
	var b strings.Builder
	b.WriteString("이전 컴패니언 대화 요약\n\n")
	if start > 0 {
		b.WriteString("- 이전 메시지 ")
		b.WriteString(strconv.Itoa(start))
		b.WriteString("개는 생략됨\n")
	}
	for _, msg := range msgs[start:] {
		text := compactSnippet(stripCompanionControlBlocks(msg.Content))
		if text == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(displayRole(msg.Role))
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func displayRole(role string) string {
	switch strings.TrimSpace(role) {
	case "assistant":
		return "컴패니언"
	case "user":
		return "나"
	default:
		if strings.TrimSpace(role) == "" {
			return "기록"
		}
		return strings.TrimSpace(role)
	}
}

func compactSnippet(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= compactHistorySnippetRunes {
		return text
	}
	return string(runes[:compactHistorySnippetRunes]) + "..."
}

func stripCompanionControlBlocks(text string) string {
	for {
		idx := firstControlFence(text)
		if idx < 0 {
			return text
		}
		rest := text[idx+len("```linetta-"):]
		end := strings.Index(rest, "```")
		if end < 0 {
			return text[:idx]
		}
		text = text[:idx] + rest[end+len("```"):]
	}
}

func firstControlFence(text string) int {
	first := -1
	for _, fence := range []string{"```linetta-proposal", "```linetta-query", "```linetta-choices"} {
		idx := strings.Index(text, fence)
		if idx >= 0 && (first < 0 || idx < first) {
			first = idx
		}
	}
	return first
}

// Send starts a companion turn; returns the run id. Streaming + proposal arrive
// via notifications.
func (s *Service) Send(ctx context.Context, projectID, nodeID, text string, now func() int64) (string, error) {
	return s.runner.start(ctx, projectID, nodeID, text, now)
}

// Cancel cancels an in-flight run.
func (s *Service) Cancel(runID string) error { return s.runner.cancel(runID) }
