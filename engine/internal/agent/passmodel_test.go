package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/tars/pkg/llm"
)

// passProviders is fakeProviders for a provider whose pass model is set.
type passProviders struct {
	client    llm.Client
	passModel string
}

func (f passProviders) Client(context.Context, string) (llm.Client, provider.Resolved, error) {
	return f.client, provider.Resolved{ID: "anthropic", Model: "big-model", PassModel: f.passModel}, nil
}

// passBuilder stands in for provider.Source.PassClient and counts its calls.
type passBuilder struct {
	mu     sync.Mutex
	client llm.Client
	err    error
	calls  int
}

func (b *passBuilder) build(context.Context, string) (llm.Client, provider.Resolved, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	return b.client, provider.Resolved{ID: "anthropic", Model: "light-model"}, b.err
}

func (b *passBuilder) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func newPassModelService(t *testing.T, providers ProviderSource, pass *passBuilder, effort string, rec *recorder) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:         providers,
		PassClient:        pass.build,
		ReasoningEffort:   func() string { return effort },
		History:           companion.NewHistoryRepo(st.DB()),
		Scope:             fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:          skillTools(&skillCounter{}),
		Notify:            rec.notify,
		Language:          func() string { return "ko" },
		SelfReviewEnabled: func() bool { return true },
		Clock:             func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// The writer's effort reaches every request of the turn, and the model's own
// default — no effort sent at all — is what an unset setting means.
func TestRun_sendsTheWritersReasoningEffort(t *testing.T) {
	for _, effort := range []string{"medium", ""} {
		rec := &recorder{}
		c := &scriptedClient{responses: []llm.ChatResponse{textReply("네")}}
		svc := newPassModelService(t, fakeProviders{client: c}, &passBuilder{}, effort, rec)
		runAndWait(t, svc, rec, "안녕")
		if got := c.options().ReasoningEffort; got != effort {
			t.Errorf("effort setting %q reached the provider as %q", effort, got)
		}
	}
}

// With a pass model set, the self-review after a working turn goes to the pass
// client and the turn itself never does.
func TestPasses_runOnThePassModelWhenOneIsSet(t *testing.T) {
	rec := &recorder{}
	turn := &scriptedClient{responses: []llm.ChatResponse{burstReply(selfReviewThreshold), textReply("끝")}}
	light := &scriptedClient{responses: []llm.ChatResponse{textReply("")}}
	pass := &passBuilder{client: light}
	svc := newPassModelService(t, passProviders{client: turn, passModel: "light-model"}, pass, "low", rec)

	runAndWait(t, svc, rec, "go")
	waitIdle(t, svc)

	if pass.count() != 1 {
		t.Fatalf("pass client built %d times, want once per turn", pass.count())
	}
	if light.count() == 0 {
		t.Fatal("the self-review never reached the pass model")
	}
	if turn.count() != 2 {
		t.Errorf("the turn's own client was called %d times, want its 2 turn requests only", turn.count())
	}
	if got := light.options().ReasoningEffort; got != "low" {
		t.Errorf("the pass ran at effort %q, want the writer's %q", got, "low")
	}
}

// No pass model, or one equal to the turn's model, builds nothing extra: the
// passes share the turn's client.
func TestPasses_shareTheTurnsClientWithoutAPassModel(t *testing.T) {
	for _, passModel := range []string{"", "big-model"} {
		rec := &recorder{}
		turn := &scriptedClient{responses: []llm.ChatResponse{burstReply(selfReviewThreshold), textReply("끝"), textReply("")}}
		pass := &passBuilder{client: &scriptedClient{responses: []llm.ChatResponse{textReply("")}}}
		svc := newPassModelService(t, passProviders{client: turn, passModel: passModel}, pass, "", rec)

		runAndWait(t, svc, rec, "go")
		waitIdle(t, svc)

		if pass.count() != 0 {
			t.Errorf("pass model %q: a pass client was built", passModel)
		}
		if turn.count() < 3 {
			t.Errorf("pass model %q: the review did not run on the turn's client (%d calls)", passModel, turn.count())
		}
	}
}

// A pass client that cannot be built costs the passes their lighter model,
// not the turn and not the review.
func TestPasses_fallBackToTheTurnsClientWhenThePassClientFails(t *testing.T) {
	rec := &recorder{}
	turn := &scriptedClient{responses: []llm.ChatResponse{burstReply(selfReviewThreshold), textReply("끝"), textReply("")}}
	pass := &passBuilder{err: errors.New("no such model")}
	svc := newPassModelService(t, passProviders{client: turn, passModel: "light-model"}, pass, "", rec)

	runAndWait(t, svc, rec, "go")
	waitIdle(t, svc)

	if !rec.has("agent.done") {
		t.Fatalf("the turn did not finish: %v", rec.methods())
	}
	if turn.count() < 3 {
		t.Errorf("the review did not fall back to the turn's client (%d calls)", turn.count())
	}
}
