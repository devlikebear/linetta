package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentusage"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/tars/pkg/llm"
)

// usageLog stands in for agentusage.Repo.
type usageLog struct {
	mu      sync.Mutex
	entries []agentusage.Entry
}

func (l *usageLog) Add(_ context.Context, e agentusage.Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	return nil
}

func (l *usageLog) byKind(kind string) []agentusage.Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []agentusage.Entry
	for _, e := range l.entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func withUsage(r llm.ChatResponse, u llm.Usage) llm.ChatResponse {
	r.Usage = u
	return r
}

// Every request leaves a row: the turn's under "turn" on the turn's model, the
// self-review's under its own kind on the pass model, each with the cache
// tokens the provider reported.
func TestUsage_recordsEachRequestOfTheTurnAndItsPasses(t *testing.T) {
	rec := &recorder{}
	log := &usageLog{}
	turn := &scriptedClient{responses: []llm.ChatResponse{
		withUsage(burstReply(selfReviewThreshold), llm.Usage{InputTokens: 12, OutputTokens: 40, CacheReadTokens: 0, CacheWriteTokens: 9000}),
		withUsage(textReply("끝"), llm.Usage{InputTokens: 30, OutputTokens: 8, CacheReadTokens: 9000, CacheWriteTokens: 600}),
	}}
	light := &scriptedClient{responses: []llm.ChatResponse{withUsage(textReply(""), llm.Usage{InputTokens: 5, OutputTokens: 2, CacheWriteTokens: 3000})}}
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:         passProviders{client: turn, passModel: "light-model"},
		PassClient:        (&passBuilder{client: light}).build,
		History:           companion.NewHistoryRepo(st.DB()),
		Scope:             fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:          skillTools(&skillCounter{}),
		Notify:            rec.notify,
		Language:          func() string { return "ko" },
		SelfReviewEnabled: func() bool { return true },
		Clock:             func() int64 { return 1700000000000 },
		Usage:             log,
	})
	t.Cleanup(func() { _ = svc.Close() })

	runAndWait(t, svc, rec, "go")
	waitIdle(t, svc)

	turns := log.byKind("turn")
	if len(turns) != 2 {
		t.Fatalf("turn rows = %+v, want one per request", turns)
	}
	for _, e := range turns {
		if e.Model != "big-model" || e.Provider != "anthropic" || e.ProjectID != "p1" || e.RunID == "" || e.At != 1700000000000 {
			t.Errorf("turn row = %+v, want the turn's provider, model, work, run and time", e)
		}
	}
	if turns[1].CacheRead != 9000 || turns[1].CacheWrite != 600 || turns[1].Input != 30 {
		t.Errorf("second turn row = %+v, want the provider's cache split kept", turns[1])
	}
	reviews := log.byKind("self_review")
	if len(reviews) != 1 || reviews[0].Model != "light-model" || reviews[0].CacheWrite != 3000 {
		t.Errorf("self-review rows = %+v, want one on the pass model", reviews)
	}
	if reviews[0].RunID != turns[0].RunID {
		t.Errorf("self-review run %q, want the turn's %q", reviews[0].RunID, turns[0].RunID)
	}
}

func TestUsageKind_namesPassesWithoutSpaces(t *testing.T) {
	for name, want := range map[string]string{
		"self-review":         "self_review",
		"settlement catch-up": "settlement_catch_up",
		"episode review":      "episode_review",
		"sample check":        "sample_check",
	} {
		if got := usageKind(name); got != want {
			t.Errorf("usageKind(%q) = %q, want %q", name, got, want)
		}
	}
}
