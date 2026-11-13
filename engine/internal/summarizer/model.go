package summarizer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
)

// The BYOK path (#135). A writer who connected a provider for the built-in
// agent can let the summarizer use it too: after a long scene is saved, the
// provider writes its summary instead of Linetta cutting the opening. It is off
// until the writer switches it on, and the provider's own consent still
// decides whether anything is sent.
//
// This package holds no model client — only internal/agent and
// internal/provider may (scripts/validate-story-core-deps.sh) — so the call
// itself is behind Model, which the agent package implements.

// ModelKind says what a model request is for.
type ModelKind string

const (
	// KindScene asks for a scene's summary.
	KindScene ModelKind = "scene"
	// KindSynopsis asks for a whole work's synopsis from its parts' summaries.
	KindSynopsis ModelKind = "synopsis"
)

// ModelRequest is one summary to write.
type ModelRequest struct {
	Kind ModelKind
	Text string
}

// ModelResult is what came back and what it cost.
type ModelResult struct {
	Text   string
	Input  int
	Output int
}

// Model writes a summary with the writer's provider.
type Model interface {
	Summarize(ctx context.Context, req ModelRequest) (ModelResult, error)
}

var (
	// ErrModelUnavailable means nothing was sent: the provider is not set up,
	// not consented to, or is one that cannot answer a plain request. A Model
	// wraps the provider's own error with it, so its reason survives.
	ErrModelUnavailable = errors.New("summarizer: provider unavailable")
	// ErrModelOff means the writer has not switched the provider summarizer on.
	ErrModelOff = errors.New("summarizer: provider summaries are off")
	// ErrNothingToSummarize means the work has no summaries to write from.
	ErrNothingToSummarize = errors.New("summarizer: nothing to summarize")
)

// maxModelInputRunes caps what one request sends. A scene longer than this is
// sent from its start: a summary of most of a very long scene beats none.
const maxModelInputRunes = 24000

// Event is one provider call, for the activity log.
type Event struct {
	Tool      string
	ProjectID string
	NodeID    string
	OK        bool
	Detail    string
}

// Activity log tool names.
const (
	ToolSummarizeScene  = "summarize_scene"
	ToolRewriteSynopsis = "rewrite_synopsis"
)

// WithModel connects the provider path. enabled is read for every scene, so
// switching it off stops the next save from sending anything. usage and record
// may be nil. Call it before Start.
func (s *Summarizer) WithModel(m Model, enabled func() bool, usage *UsageRepo, record func(context.Context, Event)) *Summarizer {
	s.model = m
	s.modelEnabled = enabled
	s.usage = usage
	s.record = record
	return s
}

func (s *Summarizer) modelOn() bool {
	return s.model != nil && s.modelEnabled != nil && s.modelEnabled()
}

// summarizeLeafWithModel asks the provider for the scene's summary. It reports
// whether the scene is dealt with; false sends the caller on to the opening
// cut, which is what happens when nothing could be sent or the provider failed.
func (s *Summarizer) summarizeLeafWithModel(ctx context.Context, projectID, nodeID, plain string, version int) bool {
	res, err := s.model.Summarize(ctx, ModelRequest{Kind: KindScene, Text: capRunes(plain, maxModelInputRunes)})
	if errors.Is(err, ErrModelUnavailable) {
		return false
	}
	if ctx.Err() != nil {
		// Shutting down. Writing anything now, even the opening cut, would
		// race the store closing; the next save summarizes the scene again.
		return true
	}
	s.addUsage(ctx, res)
	summary := strings.TrimSpace(res.Text)
	if err == nil && summary == "" {
		err = errors.New("the provider returned an empty summary")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: provider summary %s: %v\n", nodeID, err)
		s.recordError(ctx, nodeID, err.Error())
		s.recordEvent(ctx, Event{Tool: ToolSummarizeScene, ProjectID: projectID, NodeID: nodeID, Detail: err.Error()})
		return false
	}
	applied, err := s.nodes.SetGeneratedSummary(ctx, nodeID, summary, version)
	if err != nil {
		s.recordError(ctx, nodeID, err.Error())
		return true
	}
	if !applied {
		// An agent wrote the summary while the provider was busy. Theirs stays.
		return true
	}
	s.recordOK(ctx, nodeID, false)
	s.recordEvent(ctx, Event{Tool: ToolSummarizeScene, ProjectID: projectID, NodeID: nodeID, OK: true})
	return true
}

// WriteSynopsis asks the provider for a synopsis of the work, from its
// top-level parts' summaries, and returns it for the caller to save. It is the
// sidebar's rewrite button; nothing calls it on its own.
func (s *Summarizer) WriteSynopsis(ctx context.Context, projectID string) (string, error) {
	if !s.modelOn() {
		return "", ErrModelOff
	}
	source, err := s.synopsisSource(ctx, projectID)
	if err != nil {
		return "", err
	}
	if source == "" {
		return "", ErrNothingToSummarize
	}
	res, err := s.model.Summarize(ctx, ModelRequest{Kind: KindSynopsis, Text: source})
	if errors.Is(err, ErrModelUnavailable) {
		return "", err
	}
	s.addUsage(ctx, res)
	text := strings.TrimSpace(res.Text)
	if err == nil && text == "" {
		err = errors.New("the provider returned an empty synopsis")
	}
	if err != nil {
		s.recordEvent(ctx, Event{Tool: ToolRewriteSynopsis, ProjectID: projectID, Detail: err.Error()})
		return "", err
	}
	s.recordEvent(ctx, Event{Tool: ToolRewriteSynopsis, ProjectID: projectID, OK: true})
	return text, nil
}

// synopsisSource lists the work's top-level parts in order, each with its
// summary — the same roll-up a container's own summary is made of, one level
// up. Stale summaries are refreshed first without the provider.
func (s *Summarizer) synopsisSource(ctx context.Context, projectID string) (string, error) {
	all, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, n := range all {
		if n.ParentID != nil {
			continue
		}
		if !n.SummaryFresh() && !(n.Summary != "" && n.SummaryWritten) {
			s.RefreshNow(ctx, n.ID)
			if fresh, err := s.nodes.Get(ctx, n.ID); err == nil {
				n = fresh
			}
		}
		if strings.TrimSpace(n.Summary) == "" {
			continue
		}
		b.WriteString(n.Label)
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(n.Summary))
		b.WriteString("\n\n")
	}
	return capRunes(strings.TrimSpace(b.String()), maxModelInputRunes), nil
}

func (s *Summarizer) addUsage(ctx context.Context, res ModelResult) {
	if s.usage == nil || (res.Input == 0 && res.Output == 0 && res.Text == "") {
		return
	}
	if err := s.usage.Add(ctx, res.Input, res.Output); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: usage: %v\n", err)
	}
}

func (s *Summarizer) recordEvent(ctx context.Context, e Event) {
	if s.record != nil {
		s.record(ctx, e)
	}
}

func capRunes(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// Usage is what the provider path has cost, all told.
type Usage struct {
	Calls  int `json:"calls"`
	Input  int `json:"input"`
	Output int `json:"output"`
}

// UsageRepo keeps the running total in summarizer_usage.
type UsageRepo struct{ db *sql.DB }

func NewUsageRepo(db *sql.DB) *UsageRepo { return &UsageRepo{db: db} }

// Add counts one call.
func (r *UsageRepo) Add(ctx context.Context, input, output int) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO summarizer_usage (id, calls, input_tokens, output_tokens) VALUES (1, 1, ?, ?)
ON CONFLICT(id) DO UPDATE SET calls = calls + 1,
  input_tokens = input_tokens + excluded.input_tokens,
  output_tokens = output_tokens + excluded.output_tokens`, input, output)
	return err
}

// Get returns the total; zero before the first call.
func (r *UsageRepo) Get(ctx context.Context) (Usage, error) {
	var u Usage
	err := r.db.QueryRowContext(ctx,
		`SELECT calls, input_tokens, output_tokens FROM summarizer_usage WHERE id = 1`).Scan(&u.Calls, &u.Input, &u.Output)
	if errors.Is(err, sql.ErrNoRows) {
		return Usage{}, nil
	}
	return u, err
}
