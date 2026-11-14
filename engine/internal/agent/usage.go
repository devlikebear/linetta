package agent

import (
	"context"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/agentusage"
	"github.com/devlikebear/tars/pkg/llm"
)

// UsageRecorder keeps what each request cost (agentusage.Repo).
type UsageRecorder interface {
	Add(ctx context.Context, e agentusage.Entry) error
}

// usageKindTurn is every request the writer's own turn sends, its wrap-up
// included. A pass records under its own name (usageKind).
const usageKindTurn = "turn"

// usageKind turns a pass's log name ("self-review", "settlement catch-up")
// into the kind its usage rows carry ("self_review", "settlement_catch_up").
func usageKind(passName string) string {
	return strings.NewReplacer(" ", "_", "-", "_").Replace(strings.TrimSpace(passName))
}

// passerModel is the model passer runs.
func (st loopState) passerModel() string {
	if st.passClient != nil && st.passModel != "" {
		return st.passModel
	}
	return st.model
}

// recordUsage keeps one request's tokens. Best effort, and outside the turn's
// cancellation: the tokens were spent whether or not the writer stopped the
// turn after them.
func (s *Service) recordUsage(ctx context.Context, st loopState, kind, model string, u llm.Usage) {
	if s.deps.Usage == nil {
		return
	}
	err := s.deps.Usage.Add(context.WithoutCancel(ctx), agentusage.Entry{
		RunID:      st.runID,
		ProjectID:  st.projectID,
		Kind:       kind,
		Provider:   st.provider,
		Model:      model,
		Input:      u.InputTokens,
		Output:     u.OutputTokens,
		Cached:     u.CachedTokens,
		CacheRead:  u.CacheReadTokens,
		CacheWrite: u.CacheWriteTokens,
		At:         s.now(),
	})
	if err != nil {
		logf("usage for turn %s: %v", st.runID, err)
	}
}
