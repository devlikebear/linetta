package companion

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
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
const sceneExcerptMaxRunes = 1200
const sceneExcerptTotalRunes = 6000
const factContextLimit = 12
const maxImageAttachments = 4
const maxImageAttachmentBytes = 8 * 1024 * 1024

// ImageAttachment is a transient multimodal companion input. Images are sent to
// the current LLM turn but are not persisted in the transcript.
type ImageAttachment struct {
	Name      string `json:"name,omitempty"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	Size      int    `json:"size,omitempty"`
}

type SendOptions struct {
	Context          ai.ContextSelection `json:"context,omitempty"`
	OutlineStructure string              `json:"outline_structure,omitempty"`
	Intent           RequestIntent       `json:"intent,omitempty"`
	Scope            string              `json:"scope,omitempty"`
}

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
	facts         *fact.Repo
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
	history       *HistoryRepo
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

func (s *Service) WithHistory(repo *HistoryRepo) *Service {
	s.history = repo
	return s
}

func (s *Service) WithFacts(repo *fact.Repo) *Service {
	s.facts = repo
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
	if outlineNodes, err := s.loadOutlineNodes(ctx, projectID); err == nil {
		d.OutlineNodes = outlineNodes
	}
	if excerpts, err := s.loadSceneExcerpts(ctx, projectID, resolvedNode); err == nil {
		d.SceneExcerpts = excerpts
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
	if s.facts != nil {
		filter := fact.ListFilter{ProjectID: projectID, Limit: factContextLimit}
		if resolvedNode != "" {
			filter.NodeID = &resolvedNode
		}
		if facts, err := s.facts.List(ctx, filter); err == nil {
			d.Facts = facts
		}
	}
	// Keyword memory can't do topical matching (SearchExperiences matches
	// summary-contains-query), so surface the most recent facts every turn
	// rather than substring-matching the full user message. `query` is kept
	// for a future smarter (e.g. semantic) recall.
	_ = query
	d.Memories = s.Recall(projectID, "", recallLimit)
	return d, nil
}

func (s *Service) loadOutlineNodes(ctx context.Context, projectID string) ([]OutlineNode, error) {
	all, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	children := map[string][]node.Node{}
	for _, n := range all {
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	out := []OutlineNode{}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, n := range children[parent] {
			parentID := ""
			if n.ParentID != nil {
				parentID = *n.ParentID
			}
			out = append(out, OutlineNode{
				ID:       n.ID,
				ParentID: parentID,
				Kind:     n.Kind,
				Label:    n.Label,
				Title:    n.Title,
				Depth:    depth,
			})
			walk(n.ID, depth+1)
		}
	}
	walk("", 0)
	return out, nil
}

func (s *Service) loadSceneExcerpts(ctx context.Context, projectID, currentNodeID string) ([]SceneExcerpt, error) {
	all, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	leaves, byID := sceneLeavesInDocumentOrder(all)
	if len(leaves) == 0 {
		return nil, nil
	}

	ordered := make([]node.Node, 0, len(leaves))
	if currentNodeID != "" {
		for _, n := range leaves {
			if n.ID == currentNodeID {
				ordered = append(ordered, n)
				break
			}
		}
	}
	for _, n := range leaves {
		if n.ID == currentNodeID {
			continue
		}
		ordered = append(ordered, n)
	}

	out := make([]SceneExcerpt, 0, len(ordered))
	used := 0
	for _, n := range ordered {
		text := strings.TrimSpace(plainTextFromDoc(n.ContentDoc))
		if text == "" {
			continue
		}
		text = trimRunesLocal(text, sceneExcerptMaxRunes)
		remaining := sceneExcerptTotalRunes - used
		if remaining <= 0 {
			break
		}
		if len([]rune(text)) > remaining {
			text = trimRunesLocal(text, remaining)
		}
		out = append(out, SceneExcerpt{
			NodeID:    n.ID,
			Label:     node.BreadcrumbLabel(byID, n),
			Text:      text,
			IsCurrent: n.ID == currentNodeID,
		})
		used += len([]rune(text))
	}
	return out, nil
}

func sceneLeavesInDocumentOrder(all []node.Node) ([]node.Node, map[string]node.Node) {
	byID := make(map[string]node.Node, len(all))
	children := map[string][]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	leaves := []node.Node{}
	var walk func(parent string)
	walk = func(parent string) {
		for _, child := range children[parent] {
			if child.Kind == node.KindLeaf {
				leaves = append(leaves, child)
			}
			walk(child.ID)
		}
	}
	walk("")
	return leaves, byID
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
	return readSessionMessages(s.sessions.TranscriptPath(sess.ID))
}

func (s *Service) HistoryView(ctx context.Context, q HistoryQuery) ([]HistoryMessage, error) {
	if s.history == nil {
		msgs, err := s.History(ctx, q.ProjectID)
		if err != nil {
			return nil, err
		}
		return sessionMessagesToHistoryMessages(q.ProjectID, msgs), nil
	}
	if err := s.importLegacyHistoryIfNeeded(ctx, q.ProjectID); err != nil {
		return nil, err
	}
	return s.history.List(ctx, q)
}

func (s *Service) importLegacyHistoryIfNeeded(ctx context.Context, projectID string) error {
	if s.history == nil {
		return nil
	}
	count, err := s.history.ProjectMessageCount(ctx, projectID)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	msgs, err := s.History(ctx, projectID)
	if err != nil {
		return err
	}
	return s.history.ImportLegacy(ctx, projectID, msgs, time.Now().UnixMilli())
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
	msgs, err := readSessionMessages(path)
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

func (s *Service) CompactHistoryView(ctx context.Context, q HistoryQuery, now func() int64) ([]HistoryMessage, error) {
	if s.history == nil {
		msgs, err := s.CompactHistory(ctx, q.ProjectID, now)
		if err != nil {
			return nil, err
		}
		return sessionMessagesToHistoryMessages(q.ProjectID, msgs), nil
	}
	if err := s.importLegacyHistoryIfNeeded(ctx, q.ProjectID); err != nil {
		return nil, err
	}
	msgs, err := s.history.List(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	at := time.Now().UnixMilli()
	if now != nil {
		at = now()
	}
	summary := compactTranscriptSummary(historyMessagesToSessionMessages(msgs))
	if err := s.history.Clear(ctx, q); err != nil {
		return nil, err
	}
	scope := HistoryScopeProject
	nodeID := ""
	if normalizeHistoryView(q.Scope) == HistoryViewScene && strings.TrimSpace(q.NodeID) != "" {
		scope = HistoryScopeScene
		nodeID = strings.TrimSpace(q.NodeID)
	}
	compacted := HistoryMessage{
		ProjectID: q.ProjectID,
		NodeID:    nodeID,
		Role:      "assistant",
		Scope:     scope,
		Status:    HistoryStatusCompacted,
		Content:   summary,
		CreatedAt: at,
	}
	if err := s.history.Append(ctx, compacted); err != nil {
		return nil, err
	}
	return s.history.List(ctx, HistoryQuery{ProjectID: q.ProjectID, NodeID: nodeID, Scope: q.Scope, Limit: 1})
}

// ClearHistory removes all persisted companion transcript messages.
func (s *Service) ClearHistory(ctx context.Context, projectID string) error {
	sess, err := s.sessions.EnsureWorker(projectID)
	if err != nil {
		return err
	}
	return session.RewriteMessages(s.sessions.TranscriptPath(sess.ID), nil)
}

func (s *Service) ClearHistoryView(ctx context.Context, q HistoryQuery) error {
	if s.history == nil {
		return s.ClearHistory(ctx, q.ProjectID)
	}
	if normalizeHistoryView(q.Scope) == HistoryViewScene && strings.TrimSpace(q.NodeID) != "" {
		return s.history.Clear(ctx, q)
	}
	if err := s.history.Clear(ctx, HistoryQuery{ProjectID: q.ProjectID, Scope: HistoryViewProject}); err != nil {
		return err
	}
	return s.ClearHistory(ctx, q.ProjectID)
}

// DeleteProjectData removes companion files tied to a permanently deleted
// project: the hidden worker transcript and keyword memory directory.
func (s *Service) DeleteProjectData(ctx context.Context, projectID string) error {
	sess, err := s.sessions.EnsureWorker(projectID)
	if err != nil {
		return err
	}
	if err := s.sessions.Delete(sess.ID); err != nil {
		return err
	}
	return os.RemoveAll(memRoot(s.memBase, projectID))
}

// PreviewContext returns the same context sections a companion turn can inject,
// with selected flags derived from the writer's current checklist choices.
func (s *Service) PreviewContext(ctx context.Context, projectID, nodeID string, selection ai.ContextSelection) (ai.ContextPreview, error) {
	data, err := s.gatherContext(ctx, projectID, nodeID, "")
	if err != nil {
		return ai.ContextPreview{}, err
	}
	return previewFromPromptData(data, selection), nil
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

var supportedImageAttachmentMediaTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

func normalizeImageAttachments(images []ImageAttachment) ([]ImageAttachment, error) {
	if len(images) == 0 {
		return nil, nil
	}
	if len(images) > maxImageAttachments {
		return nil, fmt.Errorf("companion images: maximum %d images", maxImageAttachments)
	}
	out := make([]ImageAttachment, 0, len(images))
	for i, image := range images {
		mediaType := strings.TrimSpace(image.MediaType)
		data := strings.TrimSpace(image.Data)
		if strings.HasPrefix(data, "data:") {
			header, payload, ok := strings.Cut(data, ",")
			if !ok {
				return nil, fmt.Errorf("companion image %d: invalid data URL", i+1)
			}
			data = payload
			if mediaType == "" {
				if semi := strings.Index(header, ";"); semi >= 0 {
					mediaType = strings.TrimPrefix(header[:semi], "data:")
				}
			}
		}
		if !supportedImageAttachmentMediaTypes[mediaType] {
			return nil, fmt.Errorf("companion image %d: unsupported media type %q", i+1, mediaType)
		}
		if data == "" {
			return nil, fmt.Errorf("companion image %d: empty data", i+1)
		}
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("companion image %d: invalid base64", i+1)
		}
		if len(decoded) > maxImageAttachmentBytes {
			return nil, fmt.Errorf("companion image %d: exceeds %d bytes", i+1, maxImageAttachmentBytes)
		}
		image.MediaType = mediaType
		image.Data = data
		image.Size = len(decoded)
		out = append(out, image)
	}
	return out, nil
}

// Send starts a companion turn; returns the run id. Streaming + proposal arrive
// via notifications.
func (s *Service) Send(ctx context.Context, projectID, nodeID, text string, now func() int64) (string, error) {
	return s.SendWithContext(ctx, projectID, nodeID, text, ai.DefaultContextSelection(), now)
}

// SendWithContext starts a companion turn using the writer-selected context
// checklist state.
func (s *Service) SendWithContext(ctx context.Context, projectID, nodeID, text string, selection ai.ContextSelection, now func() int64) (string, error) {
	return s.SendWithContextAndImages(ctx, projectID, nodeID, text, selection, nil, now)
}

// SendWithContextAndImages starts a companion turn with transient multimodal
// images attached to the latest user message.
func (s *Service) SendWithContextAndImages(ctx context.Context, projectID, nodeID, text string, selection ai.ContextSelection, images []ImageAttachment, now func() int64) (string, error) {
	return s.SendWithOptionsAndImages(ctx, projectID, nodeID, text, ai.Options{Context: selection}, images, now)
}

// SendWithOptionsAndImages starts a companion turn with the full per-call
// option payload used by the desktop client.
func (s *Service) SendWithOptionsAndImages(ctx context.Context, projectID, nodeID, text string, opts ai.Options, images []ImageAttachment, now func() int64) (string, error) {
	return s.SendWithCompanionOptionsAndImages(ctx, projectID, nodeID, text, SendOptions{
		Context:          opts.Context,
		OutlineStructure: opts.OutlineStructure,
	}, images, now)
}

func (s *Service) SendWithCompanionOptionsAndImages(ctx context.Context, projectID, nodeID, text string, opts SendOptions, images []ImageAttachment, now func() int64) (string, error) {
	normalized, err := normalizeImageAttachments(images)
	if err != nil {
		return "", err
	}
	return s.runner.start(ctx, projectID, nodeID, text, opts.Context, opts.OutlineStructure, opts.Intent, opts.Scope, normalized, now)
}

// Cancel cancels an in-flight run.
func (s *Service) Cancel(runID string) error { return s.runner.cancel(runID) }
