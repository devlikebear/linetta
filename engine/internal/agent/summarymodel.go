package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/summarizer"
	"github.com/devlikebear/tars/pkg/llm"
)

// SummaryModel is the summarizer's way to the writer's provider (#135). It
// lives here because this package and internal/provider are the only ones that
// may hold a model client; the summarizer reaches it through summarizer.Model.
//
// It is one plain request with no tools and no transcript: the scene's text,
// or a work's part summaries, and a short instruction. Nothing leaves without
// the provider's consent — provider.Source.Client refuses first — and a
// self-executing provider (the Claude Code CLI) is not asked at all, since it
// runs its own agent rather than answering a request.
type SummaryModel struct {
	providers ProviderSource
	active    func() string
	timeout   time.Duration
}

// summaryTimeout bounds one request. The summarizer's worker waits on it, and
// so does engine shutdown, though shutdown cancels it first.
const summaryTimeout = 2 * time.Minute

// NewSummaryModel uses the active provider. active names it, so a
// self-executing one is recognised before a client is built.
func NewSummaryModel(providers ProviderSource, active func() string) *SummaryModel {
	return &SummaryModel{providers: providers, active: active, timeout: summaryTimeout}
}

const sceneSummaryPrompt = "Summarize the passage below in 3-5 sentences, in the same language as the passage. Keep the characters, places and key events. Add nothing that is not in it. Reply with the summary alone."

const synopsisPrompt = "Below are the summaries of a work of fiction's parts, in order. Write a synopsis of the whole work as far as it goes, in 5-8 sentences, in the same language as the summaries. Keep the main characters, the central conflict and how it has developed. Add nothing that is not in them. Reply with the synopsis alone."

// Summarize implements summarizer.Model.
func (m *SummaryModel) Summarize(ctx context.Context, req summarizer.ModelRequest) (summarizer.ModelResult, error) {
	if m.active != nil && provider.IsSelfExecuting(m.active()) {
		return summarizer.ModelResult{}, fmt.Errorf("%w: the active provider runs its own agent", summarizer.ErrModelUnavailable)
	}
	client, resolved, err := m.providers.Client(ctx, "")
	if err != nil {
		return summarizer.ModelResult{}, fmt.Errorf("%w: %w", summarizer.ErrModelUnavailable, err)
	}
	if provider.IsSelfExecuting(resolved.ID) {
		return summarizer.ModelResult{}, fmt.Errorf("%w: the active provider runs its own agent", summarizer.ErrModelUnavailable)
	}
	system := sceneSummaryPrompt
	if req.Kind == summarizer.KindSynopsis {
		system = synopsisPrompt
	}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	resp, err := client.Chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: req.Text},
	}, llm.ChatOptions{})
	out := summarizer.ModelResult{Input: resp.Usage.InputTokens, Output: resp.Usage.OutputTokens}
	if err != nil {
		return out, err
	}
	out.Text = resp.Message.Content
	return out, nil
}
