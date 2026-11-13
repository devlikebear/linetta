package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/summarizer"
	"github.com/devlikebear/tars/pkg/llm"
)

// countingProviders records whether a client was even asked for.
type countingProviders struct {
	fakeProviders
	asked int
}

func (c *countingProviders) Client(ctx context.Context, id string) (llm.Client, provider.Resolved, error) {
	c.asked++
	return c.fakeProviders.Client(ctx, id)
}

func TestSummaryModel_sendsTheSceneAndCountsTheCost(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{{
		Message: llm.ChatMessage{Role: "assistant", Content: "요약."},
		Usage:   llm.Usage{InputTokens: 300, OutputTokens: 40},
	}}}
	m := NewSummaryModel(fakeProviders{client: client}, func() string { return "anthropic" })
	got, err := m.Summarize(context.Background(), summarizer.ModelRequest{Kind: summarizer.KindScene, Text: "본문"})
	if err != nil || got != (summarizer.ModelResult{Text: "요약.", Input: 300, Output: 40}) {
		t.Fatalf("Summarize = %+v, %v", got, err)
	}
	msgs := client.messages()
	if len(msgs) != 2 || msgs[0].Content != sceneSummaryPrompt || msgs[1].Content != "본문" {
		t.Fatalf("sent %+v", msgs)
	}
	if opts := client.options(); len(opts.Tools) != 0 {
		t.Fatalf("a summary request offered tools: %+v", opts.Tools)
	}
}

func TestSummaryModel_synopsisUsesItsOwnInstruction(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{{Message: llm.ChatMessage{Content: "시놉시스"}}}}
	m := NewSummaryModel(fakeProviders{client: client}, nil)
	if _, err := m.Summarize(context.Background(), summarizer.ModelRequest{Kind: summarizer.KindSynopsis, Text: "1화\n요약"}); err != nil {
		t.Fatal(err)
	}
	if msgs := client.messages(); msgs[0].Content != synopsisPrompt {
		t.Fatalf("system = %q", msgs[0].Content)
	}
}

// Consent is the provider's rule, and it holds here: a refused client means
// nothing is sent, and the reason travels with ErrModelUnavailable.
func TestSummaryModel_withoutConsentNothingIsSent(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{{}}}
	refusal := &rpc.ReasonError{Reason: rpc.ReasonProviderConsentRequired, Err: errors.New("consent required")}
	m := NewSummaryModel(fakeProviders{client: client, err: refusal}, nil)
	_, err := m.Summarize(context.Background(), summarizer.ModelRequest{Kind: summarizer.KindScene, Text: "본문"})
	var re *rpc.ReasonError
	if !errors.Is(err, summarizer.ErrModelUnavailable) || !errors.As(err, &re) || re.Reason != rpc.ReasonProviderConsentRequired {
		t.Fatalf("err = %v", err)
	}
	if client.count() != 0 {
		t.Fatal("sent a request without consent")
	}
}

// The Claude Code CLI runs an agent of its own; it is not asked for a client.
func TestSummaryModel_skipsASelfExecutingProvider(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{{}}}
	src := &countingProviders{fakeProviders: fakeProviders{client: client}}
	m := NewSummaryModel(src, func() string { return settings.ProviderClaudeCode })
	if _, err := m.Summarize(context.Background(), summarizer.ModelRequest{Text: "본문"}); !errors.Is(err, summarizer.ErrModelUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if src.asked != 0 || client.count() != 0 {
		t.Fatalf("asked %d, sent %d", src.asked, client.count())
	}
}
